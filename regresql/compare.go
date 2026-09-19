package regresql

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type (
	CompareOptions struct {
		Root           string
		BaseURI        string
		TargetURI      string
		RunFilter      string
		Format         string // console | markdown | json
		OutputPath     string
		Warmups        int           // discarded EXPLAIN ANALYZE runs before the measured one
		Admit          bool          // preflight: exclude queries whose result isn't plan-invariant
		AdmitReps      int           // repetitions per perturbation in the admit preflight
		Samples        int           // interleaved timing runs per engine (0 = off)
		Timeout        time.Duration // per-query statement_timeout cap (0 = none)
		InjectStats    bool          // copy base stats into target so diffs are code, not ANALYZE noise
		Stability      bool          // preflight: exclude cost-tie queries whose plan swings on re-ANALYZE
		StabilityReps  int
		TieMargin      float64 // preflight: exclude queries whose runner-up plan is within this % (deterministic)
		SelfControl    bool    // calibrate against base-vs-base, suppress tiers that flag there
		SelfControlURI string  // second base instance (default: BaseURI)
	}

	// Tiers that flagged base-against-base, i.e. noise. Correctness is never
	// floored — a base-vs-base result diff is non-determinism, which --admit
	// excludes visibly.
	noiseTiers struct{ buffer, spill, qerror, shape bool }

	EngineInfo struct {
		Version    string `json:"version"`
		VersionNum int    `json:"version_num"`
	}

	// QueryComparison is the diff of one query binding between base and target.
	QueryComparison struct {
		Name    string `json:"name"`
		Binding string `json:"binding,omitempty"`

		ResultDiffer bool `json:"result_differ"`
		CriticalPlan bool `json:"critical_plan,omitempty"` // shape change of a kind that usually matters (advisory)

		PlanChanged bool             `json:"plan_changed"`
		Regressions []PlanRegression `json:"regressions,omitempty"`

		BaseBuffers   int64   `json:"base_buffers"`
		TargetBuffers int64   `json:"target_buffers"`
		BufferDelta   float64 `json:"buffer_delta_pct"`
		SpillRegress  bool    `json:"spill_regression"`

		BaseTuples   float64 `json:"base_tuples"`
		TargetTuples float64 `json:"target_tuples"`
		TupleDelta   float64 `json:"tuple_delta_pct"`

		BaseQError   float64 `json:"base_qerror"`
		TargetQError float64 `json:"target_qerror"`
		QErrorNode   string  `json:"qerror_node,omitempty"`
		QErrorWorse  bool    `json:"qerror_regression"`

		CostComparable bool    `json:"cost_comparable"`
		BaseCost       float64 `json:"base_cost"`
		TargetCost     float64 `json:"target_cost"`

		Timing *TimingResult `json:"timing,omitempty"` // advisory, only with --samples

		Severity Severity `json:"severity"`
		Note     string   `json:"note,omitempty"`
	}

	Scoreboard struct {
		Generated     string            `json:"generated"` // UTC stamp; lets a consumer reject a stale file
		Base          EngineInfo        `json:"base"`
		Target        EngineInfo        `json:"target"`
		SameVersion   bool              `json:"same_version"`
		StatsInjected bool              `json:"stats_injected,omitempty"` // base stats copied into target
		StatsWarnings int               `json:"stats_warnings,omitempty"` // pg_restore_*_stats() warning count
		GUCMismatch   []GUCDiff         `json:"guc_mismatch,omitempty"`
		Comparisons   []QueryComparison `json:"comparisons"`
		Excluded      []AdmitResult     `json:"excluded,omitempty"` // rejected by the --admit preflight
		CostTie       []AdmitResult     `json:"cost_tie,omitempty"` // EXCLUDED as cost-ties (no --self-control)

		CostTieAnnotated int `json:"cost_tie_annotated,omitempty"` // flagged noise-decided but still compared
		SelfFloored      int `json:"self_floored,omitempty"`       // bindings with a tier suppressed by --self-control
	}

	GUCDiff struct {
		Name   string `json:"name"`
		Base   string `json:"base"`
		Target string `json:"target"`
	}
)

// Severity is the ladder: higher = worse (correctness at the top).
type Severity int

const (
	SevEqual Severity = iota
	SevShape
	SevEstimation
	SevPerf
	SevIncomplete
	SevCorrectness
	SevError
)

func (s Severity) String() string {
	switch s {
	case SevShape:
		return "shape"
	case SevEstimation:
		return "estimation"
	case SevPerf:
		return "perf"
	case SevIncomplete:
		return "incomplete"
	case SevCorrectness:
		return "correctness"
	case SevError:
		return "error"
	default:
		return "equal"
	}
}

// Planner GUCs pinned identically on both servers in a fair comparison.
var plannerGUCs = []string{
	"work_mem", "random_page_cost", "seq_page_cost", "cpu_tuple_cost",
	"effective_cache_size", "default_statistics_target",
	"max_parallel_workers_per_gather", "jit",
	"enable_hashjoin", "enable_mergejoin", "enable_nestloop",
	"enable_seqscan", "enable_indexscan", "enable_material", "enable_memoize",
	// partition planning
	"enable_partitionwise_join", "enable_partitionwise_aggregate",
	"enable_partition_pruning", "enable_parallel_append",
	"plan_cache_mode", "constraint_exclusion",
	// parallelism + join order
	"min_parallel_table_scan_size", "min_parallel_index_scan_size",
	"parallel_setup_cost", "parallel_tuple_cost", "max_parallel_workers",
	"join_collapse_limit", "from_collapse_limit", "geqo", "geqo_threshold",
	"enable_incremental_sort", "enable_hashagg", "hash_mem_multiplier",
}

func Compare(opts CompareOptions) int {
	if config, err := ReadConfig(opts.Root); err == nil {
		SetGlobalConfig(config)
	}

	baseDB, err := openCompareDB(opts.BaseURI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "base: %s\n", err)
		return 2
	}
	defer baseDB.Close()

	targetDB, err := openCompareDB(opts.TargetURI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "target: %s\n", err)
		return 2
	}
	defer targetDB.Close()

	board := &Scoreboard{}
	if board.Base, err = queryEngineInfo(baseDB); err != nil {
		fmt.Fprintf(os.Stderr, "base: %s\n", err)
		return 2
	}
	if board.Target, err = queryEngineInfo(targetDB); err != nil {
		fmt.Fprintf(os.Stderr, "target: %s\n", err)
		return 2
	}
	board.SameVersion = board.Base.VersionNum == board.Target.VersionNum
	board.GUCMismatch = comparePlannerGUCs(baseDB, targetDB)

	plannedQueries, err := WalkPlans(opts.Root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to walk plans: %s\n", err)
		return 2
	}

	suite := Walk(opts.Root, nil)
	suite.SetRunFilter(opts.RunFilter)

	// preflight: drop cost-tie queries whose plan is decided by ANALYZE noise
	var unstable map[string]string
	var tieNotes map[string]string
	// deterministic cost-tie classifier: flag queries whose runner-up plan is
	// priced within a margin. Under --self-control, annotate rather than exclude
	// -- "the choice isn't cost-driven" is not "base and target disagree", which
	// --self-control measures directly.
	if opts.TieMargin > 0 {
		fmt.Fprintf(os.Stderr, "tie-margin: pricing runner-up plans on base (margin %.2f%%)…\n", opts.TieMargin)
		ties := costMarginPass(context.Background(), baseDB, plannedQueries, suite, opts.TieMargin)
		if opts.SelfControl {
			tieNotes = mergeNotes(tieNotes, ties)
			fmt.Fprintf(os.Stderr, "tie-margin: %d cost-tie(s) annotated, not excluded "+
				"(--self-control measures divergence directly)\n", len(ties))
		} else {
			unstable = mergeNotes(unstable, ties)
		}
	}
	if opts.Stability {
		reps := opts.StabilityReps
		if reps < 1 {
			reps = DefaultStabilityReps
		}
		fmt.Fprintln(os.Stderr, "stability: re-ANALYZE preflight on base…")
		sampled := stabilityPass(context.Background(), baseDB, plannedQueries, suite, reps)
		// same argument as above: an unstable plan on base says the CHOICE is
		// noise-decided, not that base and target diverge.
		if opts.SelfControl {
			tieNotes = mergeNotes(tieNotes, sampled)
			fmt.Fprintf(os.Stderr, "stability: %d unstable plan(s) annotated, not excluded "+
				"(--self-control measures divergence directly)\n", len(sampled))
		} else {
			unstable = mergeNotes(unstable, sampled)
		}
	}
	board.CostTieAnnotated = len(tieNotes)

	// give both engines identical stats so a diff is code, not ANALYZE noise
	if opts.InjectStats {
		fmt.Fprintln(os.Stderr, "inject-stats: copying base statistics into target (overwrites target stats)…")
		n, err := injectStats(opts.BaseURI, opts.TargetURI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "inject-stats: %s\n", err)
			return 2
		}
		board.StatsInjected = true
		board.StatsWarnings = n
	}

	admitReps := opts.AdmitReps
	if admitReps < 1 {
		admitReps = DefaultAdmitReps
	}

	var floor map[string]noiseTiers
	if opts.SelfControl {
		scURI := opts.SelfControlURI
		if scURI == "" {
			scURI = opts.BaseURI
		}
		scDB, err := openCompareDB(scURI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "self-control: %s\n", err)
			return 2
		}
		defer scDB.Close()
		if scURI == opts.BaseURI {
			// one instance shares cache/OIDs/plancache, so the cross-instance floor won't appear
			fmt.Fprintln(os.Stderr, "self-control: WARNING calibrating against the same instance — floor will be weak; use --self-control-uri")
		}
		if opts.InjectStats && scURI != opts.BaseURI {
			if _, err := injectStats(opts.BaseURI, scURI); err != nil {
				fmt.Fprintf(os.Stderr, "self-control inject-stats: %s\n", err)
				return 2
			}
		}
		fmt.Fprintln(os.Stderr, "self-control: diffing base against itself to establish the noise floor…")
		cal, _, _ := opts.comparePass(baseDB, scDB, plannedQueries, suite, board.SameVersion, unstable, false, admitReps, false, nil, tieNotes)
		floor = buildFloor(cal)
		board.SelfFloored = len(floor)
	}

	board.Comparisons, board.Excluded, board.CostTie =
		opts.comparePass(baseDB, targetDB, plannedQueries, suite, board.SameVersion, unstable, opts.Admit, admitReps, opts.Samples > 0, floor, tieNotes)

	if err := renderScoreboard(board, opts.Format, opts.OutputPath); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 2
	}
	return board.exitCode()
}

// comparePass diffs every admitted binding of base against target; floor is nil
// in the calibration pass.
func (opts CompareOptions) comparePass(baseDB, targetDB *sql.DB, pqs []*PlannedQuery, suite *Suite, sameVersion bool, unstable map[string]string, admit bool, admitReps int, doTiming bool, floor map[string]noiseTiers, tieNotes map[string]string) (cmps []QueryComparison, excluded, costtie []AdmitResult) {
	ctx := context.Background()
	for _, pq := range pqs {
		if !suite.matchesRunFilter(filepath.Base(pq.SQLPath), pq.Query.Name) {
			continue
		}
		if pq.Query.GetRegressQLOptions().NoTest {
			continue
		}
		timeout := resolveCompareTimeout(pq.Query, opts.Timeout)
		for _, b := range iterateBindings(pq.Plan) {
			key := bindingKey(pq.Query.Name, b.name)
			if reason, tie := unstable[key]; tie {
				costtie = append(costtie, AdmitResult{Name: pq.Query.Name, Binding: b.name, Reason: reason})
				continue
			}
			if admit {
				if ar := admitBinding(ctx, baseDB, pq.Query, b, admitReps); !ar.Admitted {
					excluded = append(excluded, ar)
					continue
				}
			}
			base := captureBinding(ctx, baseDB, pq.Query, b.bindings, timeout, opts.Warmups)
			target := captureBinding(ctx, targetDB, pq.Query, b.bindings, timeout, opts.Warmups)
			cmp := compareCaptures(pq.Query.Name, b.name, base, target, sameVersion, floor[key])
			if note, ok := tieNotes[key]; ok {
				cmp.addNote(note) // report the tie, don't suppress the row
			}
			if doTiming && cmp.Severity < SevIncomplete {
				bt, tt := sampleTiming(ctx, baseDB, targetDB, pq.Query, b.bindings, timeout, opts.Samples)
				tv := timingVerdict(bt, tt)
				cmp.Timing = &tv
			}
			cmps = append(cmps, cmp)
		}
	}
	return cmps, excluded, costtie
}

// mergeNotes folds src into dst without overwriting an existing note,
// allocating dst when nil. Either preflight can feed either map.
func mergeNotes(dst, src map[string]string) map[string]string {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]string, len(src))
	}
	for k, v := range src {
		if _, dup := dst[k]; !dup {
			dst[k] = v
		}
	}
	return dst
}

func buildFloor(cal []QueryComparison) map[string]noiseTiers {
	floor := make(map[string]noiseTiers)
	for _, c := range cal {
		nt := noiseTiers{
			buffer: c.bufferRegressed(),
			spill:  c.SpillRegress,
			qerror: c.QErrorWorse,
			shape:  c.PlanChanged,
		}
		if nt.buffer || nt.spill || nt.qerror || nt.shape {
			floor[bindingKey(c.Name, c.Binding)] = nt
		}
	}
	return floor
}

// exitCode fails on the gating signals: wrong results, perf/spill regressions,
// one-sided non-completion, or errors. Shape and q-error are reported, not gated.
func (b *Scoreboard) exitCode() int {
	for _, c := range b.Comparisons {
		if c.Severity >= SevPerf {
			return 1
		}
	}
	return 0
}

// resolveCompareTimeout: per-query metadata wins, then the --timeout flag, then
// the project config default. 0 = unbounded. A timed-out query becomes an
// incomplete divergence, not a stall.
func resolveCompareTimeout(q *Query, cliTimeout time.Duration) time.Duration {
	if opts := q.GetRegressQLOptions(); opts.Timeout > 0 {
		return opts.Timeout
	}
	if cliTimeout > 0 {
		return cliTimeout
	}
	return GetStatementTimeout()
}

type bindingRef struct {
	name     string
	bindings map[string]any
}

func iterateBindings(p *Plan) []bindingRef {
	if len(p.Query.Args) == 0 {
		return []bindingRef{{name: "", bindings: nil}}
	}
	refs := make([]bindingRef, len(p.Bindings))
	for i, b := range p.Bindings {
		refs[i] = bindingRef{name: p.Names[i], bindings: b}
	}
	return refs
}

type engineCapture struct {
	result      *ResultSet
	explain     *ExplainOutput
	timedOut    bool
	err         error
	gucsSkipped []string // surfaced as a note so a dropped GUC reaches the scoreboard
}

// captureBinding runs the query for its result set plus EXPLAIN ANALYZE for its
// plan, in one rolled-back transaction. Equal warmup on both engines (keep the
// last read, not the min) cancels cold-read/hint-bit buffer asymmetry.
func captureBinding(ctx context.Context, db *sql.DB, q *Query, bindings map[string]any, timeout time.Duration, warmups int) engineCapture {
	tx, err := db.Begin()
	if err != nil {
		return engineCapture{err: err}
	}
	defer tx.Rollback()

	if err := applyStatementTimeout(ctx, tx, timeout); err != nil {
		return engineCapture{err: err}
	}
	// before both the query and the EXPLAIN below, so they resolve names and
	// planner state identically
	if err := applySearchPath(ctx, tx, q.GetSearchPath(), true); err != nil {
		return engineCapture{err: err}
	}
	gucsSkipped := applyGUCsTx(ctx, tx, q.GetGUCs())

	sqlText := q.OrdinalQuery
	var args []any
	if len(q.Args) > 0 {
		sqlText, args = q.Prepare(bindings)
	}

	rs, err := RunQuery(ctx, tx, sqlText, args...)
	if isTimeoutError(err) {
		return engineCapture{timedOut: true, gucsSkipped: gucsSkipped}
	}
	if err != nil {
		return engineCapture{err: err, gucsSkipped: gucsSkipped}
	}

	eopts := DefaultExplainOptions()
	eopts.Analyze = true
	eopts.Buffers = true
	ex, err := warmedExplain(warmups, func() (*ExplainOutput, error) {
		return ExecuteExplainWithOptions(ctx, tx, sqlText, eopts, args...)
	})
	if isTimeoutError(err) {
		return engineCapture{result: rs, timedOut: true, gucsSkipped: gucsSkipped}
	}
	if err != nil {
		return engineCapture{result: rs, err: err, gucsSkipped: gucsSkipped}
	}
	return engineCapture{result: rs, explain: ex, gucsSkipped: gucsSkipped}
}

// sampleTiming runs EXPLAIN ANALYZE `samples` times per engine, interleaved so
// drift hits both equally. cache is already warm from the main captures
func sampleTiming(ctx context.Context, baseDB, targetDB *sql.DB, q *Query, bindings map[string]any, timeout time.Duration, samples int) (baseTimes, targetTimes []float64) {
	run := func(db *sql.DB) (float64, bool) {
		c := captureBinding(ctx, db, q, bindings, timeout, 0)
		if c.explain == nil {
			return 0, false
		}
		return c.explain.ExecutionTime, true
	}
	for i := 0; i < samples; i++ {
		if t, ok := run(baseDB); ok {
			baseTimes = append(baseTimes, t)
		}
		if t, ok := run(targetDB); ok {
			targetTimes = append(targetTimes, t)
		}
	}
	return baseTimes, targetTimes
}

// warmedExplain runs explain (warmups+1) times and returns the last (measured)
// result; a failing run stops early and returns its error.
func warmedExplain(warmups int, explain func() (*ExplainOutput, error)) (*ExplainOutput, error) {
	var ex *ExplainOutput
	var err error
	for r := 0; r <= warmups; r++ {
		if ex, err = explain(); err != nil {
			return ex, err
		}
	}
	return ex, err
}

func compareCaptures(name, binding string, base, target engineCapture, sameVersion bool, suppress noiseTiers) QueryComparison {
	c := QueryComparison{Name: name, Binding: binding}

	switch {
	case base.err != nil:
		c.Severity, c.Note = SevError, "base error: "+base.err.Error()
		return c
	case target.err != nil:
		c.Severity, c.Note = SevError, "target error: "+target.err.Error()
		return c
	case base.timedOut && target.timedOut:
		c.Severity, c.Note = SevIncomplete, "both did not complete"
		return c
	case base.timedOut:
		c.Severity, c.Note = SevIncomplete, "base did not complete (target did)"
		return c
	case target.timedOut:
		c.Severity, c.Note = SevIncomplete, "target did not complete (base did)"
		return c
	}

	sev := SevEqual

	// a dropped GUC means the query isn't planned the way its source intended
	c.addNote(gucSkipNote(base.gucsSkipped))

	// result correctness (base is the reference). A different order with an
	// identical multiset is tied rows under a non-total ORDER BY, not a wrong
	// result. report it, but never as a correctness break
	if diff := CompareResultSets(base.result, target.result, GetDiffConfig()); !diff.Identical {
		if diff.Type == DiffTypeOrdering {
			c.addNote("row order differs (tied rows); multiset identical")
		} else {
			c.ResultDiffer = true
			sev = SevCorrectness
		}
	}

	// measured actuals: target vs base
	c.BaseBuffers = rootBuffers(base.explain)
	c.TargetBuffers = rootBuffers(target.explain)
	if !suppress.buffer {
		_, c.BufferDelta = CompareBuffers(c.TargetBuffers, c.BaseBuffers, GetBufferThreshold())
		if c.bufferRegressed() {
			sev = maxSev(sev, SevPerf)
		}
	}
	if !suppress.spill {
		br, bw := SumTempBlocks(&base.explain.Plan)
		tr, tw := SumTempBlocks(&target.explain.Plan)
		bt, tt := br+bw, tr+tw
		// % gate + block floor, as with buffers
		if IsSpillRegression(tt, bt, GetBufferThreshold()) && tt-bt >= GetBufferFloor() {
			c.SpillRegress = true
			sev = maxSev(sev, SevPerf)
		}
	}

	c.BaseTuples = SumTuplesProcessed(&base.explain.Plan)
	c.TargetTuples = SumTuplesProcessed(&target.explain.Plan)
	_, c.TupleDelta = CompareTuples(c.TargetTuples, c.BaseTuples, GetBufferThreshold())

	// estimation quality
	if !suppress.qerror {
		if w := WorstQError(&base.explain.Plan); w != nil {
			c.BaseQError = w.QError
		}
		if w := WorstQError(&target.explain.Plan); w != nil {
			c.TargetQError = w.QError
			c.QErrorNode = qErrorNodeLabel(w)
		}
		if IsQErrorRegression(c.TargetQError, c.BaseQError, GetQErrorRatio(), GetQErrorFloor()) {
			c.QErrorWorse = true
			sev = maxSev(sev, SevEstimation)
		}
	}

	// cost: comparable only within a version
	c.CostComparable = sameVersion
	c.BaseCost = base.explain.Plan.TotalCost
	c.TargetCost = target.explain.Plan.TotalCost

	// plan shape; a diff at equal cost is a tiebreak, not a change (measured tiers still gate)
	if !suppress.shape {
		baseSig := ExtractPlanSignatureFromNode(&base.explain.Plan)
		targetSig := ExtractPlanSignatureFromNode(&target.explain.Plan)
		if HasPlanChanged(baseSig, targetSig) {
			if c.costTie() {
				c.addNote("plan differs at equal cost (tiebreak)")
			} else {
				c.PlanChanged = true
				c.Regressions = DetectPlanRegressions(baseSig, targetSig)
				if hasCriticalRegression(c.Regressions) {
					// shape is advisory until ties are classified reliably; flag
					// it but leave the gated tiers to correctness/buffer/spill
					c.CriticalPlan = true
					sev = maxSev(sev, SevShape)
				} else {
					sev = maxSev(sev, SevShape)
				}
			}
		}
	}

	c.Severity = sev
	return c
}

// addNote appends rather than assigns; a row can carry several observations.
func (c *QueryComparison) addNote(s string) {
	if s == "" {
		return
	}
	if c.Note != "" {
		c.Note += "; "
	}
	c.Note += s
}

const costTieTolerance = 0.01 // within 1% estimated cost = a tie

func (c QueryComparison) costTie() bool {
	if !c.CostComparable || c.BaseCost <= 0 {
		return false
	}
	d := c.TargetCost - c.BaseCost
	if d < 0 {
		d = -d
	}
	return d/c.BaseCost <= costTieTolerance
}

// injectStats copies base stats into target (pg_dump --statistics-only | psql).
// Needs pg_dump/psql on PATH; REGRESQL_PG_DUMP / REGRESQL_PSQL override.
// Returns the stats-restore warning count (0 on a clean run).
func injectStats(baseURI, targetURI string) (int, error) {
	pgDump := envOr("REGRESQL_PG_DUMP", "pg_dump")
	psql := envOr("REGRESQL_PSQL", "psql")
	cEnv := append(os.Environ(), "LC_ALL=C")

	dump := exec.Command(pgDump, "--statistics-only", baseURI)
	dump.Env = cEnv
	statsSQL, err := dump.Output()
	if err != nil {
		return 0, fmt.Errorf("pg_dump --statistics-only: %w%s", err, exitStderr(err))
	}

	apply := exec.Command(psql, "-X", "-q", "-v", "ON_ERROR_STOP=1", targetURI)
	apply.Env = cEnv
	apply.Stdin = bytes.NewReader(statsSQL)
	var stderr bytes.Buffer
	apply.Stderr = &stderr
	if err := apply.Run(); err != nil {
		return 0, fmt.Errorf("applying stats via psql: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	// pg_restore_*_stats() warns (doesn't error) on unknown relation/attribute
	n := strings.Count(stderr.String(), "WARNING")
	if n > 0 {
		fmt.Fprintf(os.Stderr, "inject-stats: %d stats-restore warning(s) — target may be missing relations/columns present in base\n", n)
	}
	return n, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func derefOr(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}

func exitStderr(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return ": " + strings.TrimSpace(string(ee.Stderr))
	}
	return ""
}

func openCompareDB(uri string) (*sql.DB, error) {
	return OpenDB(compareDSN(uri))
}

// compareDSN forces simple protocol so parallel query engages (the extended
// protocol pgx defaults to silently disables it). Only rewrites URL-form DSNs.
func compareDSN(uri string) string {
	if !strings.Contains(uri, "://") || strings.Contains(uri, "default_query_exec_mode") {
		return uri
	}
	sep := "?"
	if strings.Contains(uri, "?") {
		sep = "&"
	}
	return uri + sep + "default_query_exec_mode=simple_protocol"
}

func queryEngineInfo(db *sql.DB) (EngineInfo, error) {
	var info EngineInfo
	err := db.QueryRow("SELECT current_setting('server_version'), current_setting('server_version_num')::int").
		Scan(&info.Version, &info.VersionNum)
	return info, err
}

func comparePlannerGUCs(baseDB, targetDB *sql.DB) []GUCDiff {
	var diffs []GUCDiff
	for _, name := range plannerGUCs {
		// true = missing_ok, so a GUC absent on one build reads as NULL not error
		var b, t *string
		if baseDB.QueryRow("SELECT current_setting($1, true)", name).Scan(&b) != nil {
			continue
		}
		if targetDB.QueryRow("SELECT current_setting($1, true)", name).Scan(&t) != nil {
			continue
		}
		bv, tv := derefOr(b, "(absent)"), derefOr(t, "(absent)")
		if bv != tv {
			diffs = append(diffs, GUCDiff{Name: name, Base: bv, Target: tv})
		}
	}
	return diffs
}

// bufferRegressed gates on percentage AND an absolute block floor, so a tiny
// diff on a cheap query (a few blocks) can't post a huge percentage.
func (c QueryComparison) bufferRegressed() bool {
	if c.BaseBuffers == 0 { // percentage is undefined; gate on the floor alone
		return c.TargetBuffers >= GetBufferFloor()
	}
	return c.BufferDelta > GetBufferThreshold() && c.TargetBuffers-c.BaseBuffers >= GetBufferFloor()
}

func rootBuffers(e *ExplainOutput) int64 {
	p := e.Plan
	return p.SharedHitBlocks + p.SharedReadBlocks + p.LocalHitBlocks + p.LocalReadBlocks
}

func maxSev(a, b Severity) Severity {
	if a > b {
		return a
	}
	return b
}
