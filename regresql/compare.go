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
		Root        string
		BaseURI     string
		TargetURI   string
		RunFilter   string
		Format      string // console | markdown | json
		OutputPath  string
		Warmups     int           // discarded EXPLAIN ANALYZE runs before the measured one
		Admit       bool          // preflight: exclude queries whose result isn't plan-invariant
		AdmitReps   int           // repetitions per perturbation in the admit preflight
		Samples     int           // interleaved timing runs per engine (0 = off)
		Timeout     time.Duration // per-query statement_timeout cap (0 = none)
		InjectStats bool          // copy base stats into target so diffs are code, not ANALYZE noise
	}

	EngineInfo struct {
		Version    string `json:"version"`
		VersionNum int    `json:"version_num"`
	}

	// QueryComparison is the diff of one query binding between base and target.
	QueryComparison struct {
		Name    string `json:"name"`
		Binding string `json:"binding,omitempty"`

		ResultDiffer bool `json:"result_differ"`

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
		Base          EngineInfo        `json:"base"`
		Target        EngineInfo        `json:"target"`
		SameVersion   bool              `json:"same_version"`
		StatsInjected bool              `json:"stats_injected,omitempty"` // base stats copied into target
		GUCMismatch   []GUCDiff         `json:"guc_mismatch,omitempty"`
		Comparisons   []QueryComparison `json:"comparisons"`
		Excluded      []AdmitResult     `json:"excluded,omitempty"` // rejected by the --admit preflight
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

	// give both engines identical stats so a diff is code, not ANALYZE noise
	if opts.InjectStats {
		fmt.Fprintln(os.Stderr, "inject-stats: copying base statistics into target (overwrites target stats)…")
		if err := injectStats(opts.BaseURI, opts.TargetURI); err != nil {
			fmt.Fprintf(os.Stderr, "inject-stats: %s\n", err)
			return 2
		}
		board.StatsInjected = true
	}

	plannedQueries, err := WalkPlans(opts.Root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to walk plans: %s\n", err)
		return 2
	}

	suite := Walk(opts.Root, nil)
	suite.SetRunFilter(opts.RunFilter)

	admitReps := opts.AdmitReps
	if admitReps < 1 {
		admitReps = DefaultAdmitReps
	}

	for _, pq := range plannedQueries {
		if !suite.matchesRunFilter(filepath.Base(pq.SQLPath), pq.Query.Name) {
			continue
		}
		if pq.Query.GetRegressQLOptions().NoTest {
			continue
		}
		timeout := resolveCompareTimeout(pq.Query, opts.Timeout)
		for _, b := range iterateBindings(pq.Plan) {
			// exclude plan-dependent queries: their diff would be a false signal
			if opts.Admit {
				if ar := admitBinding(context.Background(), baseDB, pq.Query, b, admitReps); !ar.Admitted {
					board.Excluded = append(board.Excluded, ar)
					continue
				}
			}
			base := captureBinding(context.Background(), baseDB, pq.Query, b.bindings, timeout, opts.Warmups)
			target := captureBinding(context.Background(), targetDB, pq.Query, b.bindings, timeout, opts.Warmups)
			cmp := compareCaptures(pq.Query.Name, b.name, base, target, board.SameVersion)
			// timing is the softest tier, only for queries that ran both sides
			if opts.Samples > 0 && cmp.Severity < SevIncomplete {
				bt, tt := sampleTiming(context.Background(), baseDB, targetDB, pq.Query, b.bindings, timeout, opts.Samples)
				tv := timingVerdict(bt, tt)
				cmp.Timing = &tv
			}
			board.Comparisons = append(board.Comparisons, cmp)
		}
	}

	if err := renderScoreboard(board, opts.Format, opts.OutputPath); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 2
	}
	return board.exitCode()
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
	result   *ResultSet
	explain  *ExplainOutput
	timedOut bool
	err      error
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

	sqlText := q.OrdinalQuery
	var args []any
	if len(q.Args) > 0 {
		sqlText, args = q.Prepare(bindings)
	}

	rs, err := RunQuery(ctx, tx, sqlText, args...)
	if isTimeoutError(err) {
		return engineCapture{timedOut: true}
	}
	if err != nil {
		return engineCapture{err: err}
	}

	eopts := DefaultExplainOptions()
	eopts.Analyze = true
	eopts.Buffers = true
	ex, err := warmedExplain(warmups, func() (*ExplainOutput, error) {
		return ExecuteExplainWithOptions(ctx, tx, sqlText, eopts, args...)
	})
	if isTimeoutError(err) {
		return engineCapture{result: rs, timedOut: true}
	}
	if err != nil {
		return engineCapture{result: rs, err: err}
	}
	return engineCapture{result: rs, explain: ex}
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

func compareCaptures(name, binding string, base, target engineCapture, sameVersion bool) QueryComparison {
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

	// result correctness (base is the reference)
	if diff := CompareResultSets(base.result, target.result, GetDiffConfig()); !diff.Identical {
		c.ResultDiffer = true
		sev = SevCorrectness
	}

	// measured actuals: target vs base
	c.BaseBuffers = rootBuffers(base.explain)
	c.TargetBuffers = rootBuffers(target.explain)
	bufferOk, delta := CompareBuffers(c.TargetBuffers, c.BaseBuffers, GetBufferThreshold())
	c.BufferDelta = delta
	c.SpillRegress = IsSpillRegression(rootTemp(target.explain), rootTemp(base.explain), GetBufferThreshold())
	if !bufferOk || c.SpillRegress {
		sev = maxSev(sev, SevPerf)
	}

	c.BaseTuples = SumTuplesProcessed(&base.explain.Plan)
	c.TargetTuples = SumTuplesProcessed(&target.explain.Plan)
	_, c.TupleDelta = CompareTuples(c.TargetTuples, c.BaseTuples, GetBufferThreshold())

	// estimation quality
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

	// plan shape
	baseSig := ExtractPlanSignatureFromNode(&base.explain.Plan)
	targetSig := ExtractPlanSignatureFromNode(&target.explain.Plan)
	if HasPlanChanged(baseSig, targetSig) {
		c.PlanChanged = true
		c.Regressions = DetectPlanRegressions(baseSig, targetSig)
		if hasCriticalRegression(c.Regressions) {
			sev = maxSev(sev, SevPerf)
		} else {
			sev = maxSev(sev, SevShape)
		}
	}

	// cost: shown only when the versions match (cost model changes between releases)
	c.CostComparable = sameVersion
	c.BaseCost = base.explain.Plan.TotalCost
	c.TargetCost = target.explain.Plan.TotalCost

	c.Severity = sev
	return c
}

// injectStats copies base stats into target (pg_dump --statistics-only | psql).
// Needs pg_dump/psql on PATH; REGRESQL_PG_DUMP / REGRESQL_PSQL override.
func injectStats(baseURI, targetURI string) error {
	pgDump := envOr("REGRESQL_PG_DUMP", "pg_dump")
	psql := envOr("REGRESQL_PSQL", "psql")

	dump := exec.Command(pgDump, "--statistics-only", baseURI)
	statsSQL, err := dump.Output()
	if err != nil {
		return fmt.Errorf("pg_dump --statistics-only: %w%s", err, exitStderr(err))
	}

	apply := exec.Command(psql, "-q", "-v", "ON_ERROR_STOP=1", targetURI)
	apply.Stdin = bytes.NewReader(statsSQL)
	var stderr bytes.Buffer
	apply.Stderr = &stderr
	if err := apply.Run(); err != nil {
		return fmt.Errorf("applying stats via psql: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
		var b, t string
		if baseDB.QueryRow("SELECT current_setting($1)", name).Scan(&b) != nil {
			continue
		}
		if targetDB.QueryRow("SELECT current_setting($1)", name).Scan(&t) != nil {
			continue
		}
		if b != t {
			diffs = append(diffs, GUCDiff{Name: name, Base: b, Target: t})
		}
	}
	return diffs
}

func rootBuffers(e *ExplainOutput) int64 {
	p := e.Plan
	return p.SharedHitBlocks + p.SharedReadBlocks + p.LocalHitBlocks + p.LocalReadBlocks
}

func rootTemp(e *ExplainOutput) int64 {
	return e.Plan.TempReadBlocks + e.Plan.TempWrittenBlocks
}

func maxSev(a, b Severity) Severity {
	if a > b {
		return a
	}
	return b
}
