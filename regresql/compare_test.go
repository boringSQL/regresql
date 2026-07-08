package regresql

import (
	"encoding/json"
	"errors"
	"testing"
)

func rowsCapture(explainRaw string, cols []string, rows [][]any) engineCapture {
	return engineCapture{
		result:  &ResultSet{Cols: cols, Rows: rows},
		explain: mustExplain(explainRaw),
	}
}

func mustExplain(raw string) *ExplainOutput {
	var out ExplainOutput
	json.Unmarshal([]byte(raw), &out)
	return &out
}

// A clean plan node: cheap, well-estimated, no I/O worth flagging.
const cleanPlan = `{"Plan":{"Node Type":"Seq Scan","Relation Name":"t","Plan Rows":100,"Actual Rows":100,"Actual Loops":1,"Shared Hit Blocks":10,"Total Cost":5}}`

// Identical results and identical plans on both engines: nothing to report.
func TestCompareCaptures_Equal(t *testing.T) {
	base := rowsCapture(cleanPlan, []string{"n"}, [][]any{{int64(1)}})
	target := rowsCapture(cleanPlan, []string{"n"}, [][]any{{int64(1)}})

	c := compareCaptures("q", "", base, target, true)
	if c.Severity != SevEqual {
		t.Errorf("severity = %v, want equal", c.Severity)
	}
	if c.ResultDiffer || c.PlanChanged || c.SpillRegress || c.QErrorWorse {
		t.Errorf("expected no flags set, got %+v", c)
	}
}

// Different rows for the same query is the top of the ladder — a wrong-results
// break outranks any perf signal captured alongside it.
func TestCompareCaptures_CorrectnessWins(t *testing.T) {
	base := rowsCapture(cleanPlan, []string{"n"}, [][]any{{int64(1)}})
	// target returns different rows AND touches many more buffers
	target := rowsCapture(
		`{"Plan":{"Node Type":"Seq Scan","Plan Rows":100,"Actual Rows":100,"Actual Loops":1,"Shared Hit Blocks":99999}}`,
		[]string{"n"}, [][]any{{int64(2)}})

	c := compareCaptures("q", "", base, target, true)
	if !c.ResultDiffer {
		t.Error("ResultDiffer = false, want true")
	}
	if c.Severity != SevCorrectness {
		t.Errorf("severity = %v, want correctness (must outrank the buffer regression)", c.Severity)
	}
}

// Same results, but the target reads far more buffers: a perf regression that
// gates the run.
func TestCompareCaptures_BufferRegression(t *testing.T) {
	base := rowsCapture(cleanPlan, []string{"n"}, [][]any{{int64(1)}})
	target := rowsCapture(
		`{"Plan":{"Node Type":"Seq Scan","Relation Name":"t","Plan Rows":100,"Actual Rows":100,"Actual Loops":1,"Shared Hit Blocks":1000}}`,
		[]string{"n"}, [][]any{{int64(1)}})

	c := compareCaptures("q", "", base, target, true)
	if c.Severity != SevPerf {
		t.Errorf("severity = %v, want perf", c.Severity)
	}
	if c.BufferDelta <= GetBufferThreshold() {
		t.Errorf("BufferDelta = %.1f, want a large increase", c.BufferDelta)
	}
}

// Target starts spilling to disk (temp blocks appear) even though shared buffers
// are fine: still a perf regression.
func TestCompareCaptures_SpillRegression(t *testing.T) {
	base := rowsCapture(cleanPlan, []string{"n"}, [][]any{{int64(1)}})
	target := rowsCapture(
		`{"Plan":{"Node Type":"Sort","Plan Rows":100,"Actual Rows":100,"Actual Loops":1,"Shared Hit Blocks":10,"Temp Written Blocks":500}}`,
		[]string{"n"}, [][]any{{int64(1)}})

	c := compareCaptures("q", "", base, target, true)
	if !c.SpillRegress {
		t.Error("SpillRegress = false, want true")
	}
	if c.Severity != SevPerf {
		t.Errorf("severity = %v, want perf", c.Severity)
	}
}

// The target's worst-node estimate blows up relative to base: an estimation
// regression, reported but below the perf gate.
func TestCompareCaptures_QErrorRegression(t *testing.T) {
	// base: estimate spot-on (q-error ~1)
	base := rowsCapture(cleanPlan, []string{"n"}, [][]any{{int64(1)}})
	// target: estimates 1 row, gets 1000 (q-error 1000)
	target := rowsCapture(
		`{"Plan":{"Node Type":"Seq Scan","Relation Name":"t","Plan Rows":1,"Actual Rows":1000,"Actual Loops":1,"Shared Hit Blocks":10}}`,
		[]string{"n"}, [][]any{{int64(1)}})

	c := compareCaptures("q", "", base, target, true)
	if !c.QErrorWorse {
		t.Errorf("QErrorWorse = false; base=%.0f target=%.0f", c.BaseQError, c.TargetQError)
	}
	if c.Severity != SevEstimation {
		t.Errorf("severity = %v, want estimation", c.Severity)
	}
}

// A one-sided timeout is a divergence in its own right, not a fatal error.
func TestCompareCaptures_OneSidedTimeout(t *testing.T) {
	base := rowsCapture(cleanPlan, []string{"n"}, [][]any{{int64(1)}})
	target := engineCapture{timedOut: true}

	c := compareCaptures("q", "", base, target, true)
	if c.Severity != SevIncomplete {
		t.Errorf("severity = %v, want incomplete", c.Severity)
	}
}

// Cost is comparable only within a single server version; across versions the
// cost model itself changes, so the flag must be off.
func TestCompareCaptures_CostSuppressedAcrossVersions(t *testing.T) {
	base := rowsCapture(cleanPlan, []string{"n"}, [][]any{{int64(1)}})
	target := rowsCapture(cleanPlan, []string{"n"}, [][]any{{int64(1)}})

	same := compareCaptures("q", "", base, target, true)
	if !same.CostComparable {
		t.Error("CostComparable = false for same version, want true")
	}
	cross := compareCaptures("q", "", base, target, false)
	if cross.CostComparable {
		t.Error("CostComparable = true across versions, want false")
	}
}

// exitCode fails (1) on perf-and-above, passes (0) on shape/estimation only.
func TestScoreboardExitCode(t *testing.T) {
	cases := []struct {
		name string
		sev  Severity
		want int
	}{
		{"equal", SevEqual, 0},
		{"shape only", SevShape, 0},
		{"estimation only", SevEstimation, 0},
		{"perf", SevPerf, 1},
		{"incomplete", SevIncomplete, 1},
		{"correctness", SevCorrectness, 1},
		{"error", SevError, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &Scoreboard{Comparisons: []QueryComparison{{Severity: tc.sev}}}
			if got := b.exitCode(); got != tc.want {
				t.Errorf("exitCode = %d, want %d", got, tc.want)
			}
		})
	}
}

// totals tallies the cover-letter counters, including q-error direction:
// a smaller target q-error counts as improved, the QErrorWorse flag as regressed.
func TestScoreboardTotals(t *testing.T) {
	b := &Scoreboard{Comparisons: []QueryComparison{
		{ResultDiffer: true, Severity: SevCorrectness},
		{PlanChanged: true, Severity: SevShape},
		{SpillRegress: true, Severity: SevPerf},
		{BaseQError: 100, TargetQError: 5, Severity: SevEqual},                         // improved
		{BaseQError: 2, TargetQError: 340, QErrorWorse: true, Severity: SevEstimation}, // regressed
	}}

	tot := b.totals()
	if tot.Queries != 5 {
		t.Errorf("Queries = %d, want 5", tot.Queries)
	}
	if tot.Correctness != 1 || tot.Shape != 1 || tot.Spill != 1 {
		t.Errorf("correctness/shape/spill = %d/%d/%d, want 1/1/1", tot.Correctness, tot.Shape, tot.Spill)
	}
	if tot.QErrImproved != 1 || tot.QErrRegressed != 1 {
		t.Errorf("q-error +%d/-%d, want +1/-1", tot.QErrImproved, tot.QErrRegressed)
	}
}

// warmedExplain runs explain warmups+1 times (warmups discarded + one measured)
// and returns the LAST result — the steady-state read that cancels cold-cache skew.
func TestWarmedExplain_RunsAndKeepsLast(t *testing.T) {
	cases := []struct{ warmups, wantRuns int }{
		{0, 1}, // no warmup: a single measured run
		{2, 3}, // 2 discarded + 1 measured
		{4, 5},
	}
	for _, tc := range cases {
		runs := 0
		ex, err := warmedExplain(tc.warmups, func() (*ExplainOutput, error) {
			runs++
			// tag each run so we can prove the LAST one is returned, not the first
			return &ExplainOutput{PlanningTime: float64(runs)}, nil
		})
		if err != nil {
			t.Fatalf("warmups=%d: %v", tc.warmups, err)
		}
		if runs != tc.wantRuns {
			t.Errorf("warmups=%d ran %d times, want %d", tc.warmups, runs, tc.wantRuns)
		}
		if ex.PlanningTime != float64(tc.wantRuns) {
			t.Errorf("warmups=%d returned run %v, want the last (%d)", tc.warmups, ex.PlanningTime, tc.wantRuns)
		}
	}
}

// A failing run stops the warm loop early and surfaces its error (so a timeout on
// a warm-up isn't masked by a later successful run).
func TestWarmedExplain_StopsOnError(t *testing.T) {
	runs := 0
	_, err := warmedExplain(5, func() (*ExplainOutput, error) {
		runs++
		if runs == 2 {
			return nil, errors.New("statement timeout")
		}
		return &ExplainOutput{}, nil
	})
	if err == nil {
		t.Fatal("warmedExplain = nil error, want the run-2 error")
	}
	if runs != 2 {
		t.Errorf("ran %d times, want it to stop at 2", runs)
	}
}

// compareDSN forces simple protocol on URL DSNs (so parallel query engages) but
// leaves keyword DSNs and already-configured URLs untouched.
func TestCompareDSN(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"bare url", "postgres://u@h/db", "postgres://u@h/db?default_query_exec_mode=simple_protocol"},
		{"url with param", "postgres://u@h/db?sslmode=disable", "postgres://u@h/db?sslmode=disable&default_query_exec_mode=simple_protocol"},
		{"already set", "postgres://u@h/db?default_query_exec_mode=exec", "postgres://u@h/db?default_query_exec_mode=exec"},
		{"keyword dsn untouched", "host=h user=u dbname=db", "host=h user=u dbname=db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compareDSN(tc.in); got != tc.want {
				t.Errorf("compareDSN(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
