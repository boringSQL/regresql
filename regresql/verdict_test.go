package regresql

import "testing"

var (
	criticalReg = []PlanRegression{{Severity: "critical", Message: "seq scan"}}
	warningReg  = []PlanRegression{{Severity: "warning", Message: "shape"}}
)

func TestGateVerdictBuffersMatchesLegacy(t *testing.T) {
	for _, branchOk := range []bool{false, true} {
		for _, regs := range [][]PlanRegression{nil, warningReg, criticalReg} {
			for _, buf := range []bool{false, true} {
				got, notes := gateVerdict(verdictInput{
					Mode: ModeBuffers, BranchOk: branchOk,
					HasSignature: regs != nil, PlanChanged: regs != nil, Regressions: regs,
				})
				want := branchOk
				if hasCriticalRegression(regs) {
					want = false
				}
				if got != want || len(notes) != 0 {
					t.Errorf("branchOk=%v regs=%v buf=%v: got %v notes=%v, want %v", branchOk, regs, buf, got, notes, want)
				}
			}
		}
	}
}

func TestGateVerdictBuffersPlanChangedAlonePasses(t *testing.T) {
	ok, _ := gateVerdict(verdictInput{Mode: ModeBuffers, BranchOk: true, HasSignature: true, PlanChanged: true})
	if !ok {
		t.Error("buffers mode must not gate on plan change alone")
	}
}

func TestGateVerdictPlanChoice(t *testing.T) {
	tests := []struct {
		name      string
		in        verdictInput
		wantOk    bool
		wantNotes int
	}{
		{"cost blowup same shape passes", verdictInput{BranchOk: false, HasSignature: true}, true, 0},
		{"branch ok ignored on shape change", verdictInput{BranchOk: true, HasSignature: true, PlanChanged: true}, false, 0},
		{"changed with empty regressions fails", verdictInput{HasSignature: true, PlanChanged: true}, false, 0},
		{"non critical regression with change fails", verdictInput{HasSignature: true, PlanChanged: true, Regressions: warningReg}, false, 0},
		{"critical fails", verdictInput{BranchOk: true, HasSignature: true, Regressions: criticalReg}, false, 0},
		{"no signature passes with note", verdictInput{BranchOk: false}, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Mode = ModePlanChoice
			ok, notes := gateVerdict(tt.in)
			if ok != tt.wantOk || len(notes) != tt.wantNotes {
				t.Errorf("got ok=%v notes=%d, want ok=%v notes=%d", ok, len(notes), tt.wantOk, tt.wantNotes)
			}
			for _, n := range notes {
				if n.Type != NoPlanSignature || n.Severity != "info" {
					t.Errorf("unexpected note %+v", n)
				}
			}
		})
	}
}

func TestResultNameFormatsUnchanged(t *testing.T) {
	r := &TestResult{ActualCost: 12.5, ExpectedCost: 10, Threshold: 10}
	st, name := legacyCostResult("q.cost", true, r, 25)
	if st != "passed" || name != "q.cost (12.50 <= 10.00 * 110%)" {
		t.Errorf("legacy pass: %s %q", st, name)
	}
	st, name = legacyCostResult("q.cost", false, r, 25)
	if st != "failed" || name != "q.cost (12.50 > 10.00 * 110%, +25.0%)" {
		t.Errorf("legacy fail: %s %q", st, name)
	}
	st, name = planChoiceResult("q.cost", true, r)
	if st != "passed" || name != "q.cost (shape unchanged; cost 12.50 vs 10.00, advisory)" {
		t.Errorf("plan choice pass: %s %q", st, name)
	}
	st, name = planChoiceResult("q.cost", false, r)
	if st != "failed" || name != "q.cost (plan changed)" {
		t.Errorf("plan choice fail: %s %q", st, name)
	}
}

func TestBufferResultGoldenNames(t *testing.T) {
	base := TestResult{
		ActualBuffers: 150, BaselineBuffers: 100, Threshold: 2,
		ActualTempBuffers: 8, BaselineTempBuffers: 0,
		ActualQError: 40, BaselineQError: 5, QErrorNode: "Seq Scan on t",
		TupleIncrease: 33.3,
	}
	tests := []struct {
		name         string
		mut          func(r *TestResult)
		ok, bufferOk bool
		wantStatus   string
		want         string
	}{
		{"pass", func(r *TestResult) {}, true, true, "passed", "q.buffers (150 <= 100 * 102%)"},
		{"spill", func(r *TestResult) { r.SpillRegression = true }, false, true, "failed", "q.buffers (spill: temp 8 > 0 blocks)"},
		{"qerror only", func(r *TestResult) { r.QErrorRegression = true }, false, true, "failed", "q.buffers (q-error 40x > baseline 5x, Seq Scan on t)"},
		{"default fail", func(r *TestResult) {}, false, false, "failed", "q.buffers (150 > 100 * 102%, +50.0%)"},
		{"pass with tuples", func(r *TestResult) { r.TupleRegression = true }, true, true, "passed", "q.buffers (150 <= 100 * 102%) [tuples +33.3%]"},
		{"pass with qerror suffix", func(r *TestResult) { r.QErrorRegression = true }, true, true, "passed", "q.buffers (150 <= 100 * 102%) [q-error 40x > 5x]"},
		{"fail with both suffixes", func(r *TestResult) { r.TupleRegression = true; r.QErrorRegression = true }, false, false, "failed", "q.buffers (150 > 100 * 102%, +50.0%) [tuples +33.3%] [q-error 40x > 5x]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			tt.mut(&r)
			status, name := bufferResult("q.buffers", tt.ok, tt.bufferOk, &r, 50)
			if status != tt.wantStatus || name != tt.want {
				t.Errorf("got %s %q, want %s %q", status, name, tt.wantStatus, tt.want)
			}
		})
	}
}
