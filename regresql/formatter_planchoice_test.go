package regresql

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mndrix/tap-go"
)

func planChoiceFailure(regs ...PlanRegression) TestResult {
	return TestResult{
		Name: "q.cost (plan changed)", Type: "cost", Status: "failed",
		ExpectedCost: 10, ActualCost: 10, PlanChoice: true, PlanChanged: true, PlanRegressions: regs,
	}
}

func TestConsolePlanChoiceSignatureChangedLine(t *testing.T) {
	f := &ConsoleFormatter{}
	var buf bytes.Buffer
	f.printCostFailure(planChoiceFailure(), &buf)
	out := buf.String()
	if !strings.Contains(out, "Plan signature changed") || !strings.Contains(out, "regresql baseline") {
		t.Errorf("missing explicit signature line: %q", out)
	}
	if strings.Contains(out, "ANALYZE table_name") {
		t.Errorf("ANALYZE hint must be suppressed: %q", out)
	}
	if !strings.Contains(out, "Cost (info)") {
		t.Errorf("cost must be informational: %q", out)
	}
}

func TestConsolePlanChoiceWithRegressions(t *testing.T) {
	f := &ConsoleFormatter{}
	var buf bytes.Buffer
	f.printCostFailure(planChoiceFailure(PlanRegression{Severity: "critical", Table: "orders", OldScan: "Index Scan", NewScan: "Seq Scan"}), &buf)
	out := buf.String()
	if !strings.Contains(out, "Plan shape changed") || !strings.Contains(out, "orders") {
		t.Errorf("unexpected output: %q", out)
	}
	if strings.Contains(out, "Plan signature changed") {
		t.Errorf("signature line only when no regressions: %q", out)
	}
}

func TestConsoleBuffersCostFailureUntouched(t *testing.T) {
	f := &ConsoleFormatter{}
	var buf bytes.Buffer
	f.printCostFailure(TestResult{Type: "cost", ExpectedCost: 10, ActualCost: 20, PercentIncrease: 100}, &buf)
	want := "  Expected cost: 10.00\n  Actual cost:   20.00 (+100.0%)\n\n  Likely cause: Data distribution changed or outdated statistics\n  Try: ANALYZE table_name;\n"
	if buf.String() != want {
		t.Errorf("got %q", buf.String())
	}
}

func TestConsoleWarningsListsInfoCostIncrease(t *testing.T) {
	f := &ConsoleFormatter{}
	r := TestResult{
		Name: "q.cost (shape unchanged; cost 60.00 vs 10.00, advisory)", Type: "cost", Status: "passed", PlanChoice: true,
		PlanWarnings: []PlanWarning{{Type: CostIncreased, Severity: "info", Message: "Cost 60.00 vs baseline 10.00 (+500.0%), plan shape unchanged"}},
	}
	f.results = []TestResult{r}
	var buf bytes.Buffer
	s := NewTestSummary()
	s.AddResult(r)
	if err := f.Finish(s, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "WARNINGS:") || !strings.Contains(out, "+500.0%") {
		t.Errorf("info warning missing from WARNINGS block: %q", out)
	}
}

func TestJUnitPlanChoiceMessage(t *testing.T) {
	f := &JUnitFormatter{}
	_ = f.AddResult(planChoiceFailure(PlanRegression{Severity: "warning", Message: "join changed"}), nil)
	var buf bytes.Buffer
	if err := f.Finish(NewTestSummary(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `message="Plan changed"`) || strings.Contains(out, "Cost increased") || !strings.Contains(out, "join changed") {
		t.Errorf("unexpected junit: %s", out)
	}
}

func TestPgTAPPlanChoiceMessage(t *testing.T) {
	var buf bytes.Buffer
	f := &PgTAPFormatter{t: tap.New()}
	f.t.Writer = &buf
	if err := f.AddResult(planChoiceFailure(PlanRegression{Severity: "warning", Message: "join changed"}), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Plan changed") || strings.Contains(out, "Cost increased") || !strings.Contains(out, "join changed") {
		t.Errorf("unexpected tap: %s", out)
	}
}

func TestGitHubPlanChoiceMessage(t *testing.T) {
	var buf bytes.Buffer
	f := &GitHubActionsFormatter{}
	if err := f.AddResult(planChoiceFailure(PlanRegression{Severity: "critical", Table: "orders", OldScan: "Index Scan", NewScan: "Seq Scan"}), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "::error::Plan changed in") || strings.Contains(out, "Cost regression") || !strings.Contains(out, "orders") {
		t.Errorf("unexpected github output: %s", out)
	}
}

func TestPolicyEscalatesCostIncreaseToViolation(t *testing.T) {
	r := TestResult{PlanWarnings: []PlanWarning{{Type: CostIncreased, Severity: "info"}}}
	if hasSeverityViolation([]TestResult{r}, true) {
		t.Fatal("info must not violate without a policy")
	}
	ApplyPolicies(&r, &PoliciesConfig{Severity: map[string]string{"cost_increase": "error"}})
	if !hasSeverityViolation([]TestResult{r}, false) {
		t.Error("escalated cost_increase must trigger a violation")
	}
}

func TestPlanChoiceSeqScanWarningCanFailStrict(t *testing.T) {
	sig := buildSigWithSeqScans("orders")
	n := &PlanNode{TotalCost: 1000}
	if got := planCostInfo(n, false).TotalBuffers; got != 0 {
		t.Fatalf("buffers mode keeps HEAD value 0, got %d", got)
	}
	info := planCostInfo(n, true)
	if info.TotalBuffers != -1 {
		t.Fatalf("plan choice must report buffers unavailable, got %d", info.TotalBuffers)
	}
	r := TestResult{Type: "cost", Status: "passed", PlanChoice: true,
		PlanWarnings: DetectPlanQualityIssues(sig, RegressQLOptions{}, nil, nil, info)}
	if !hasSeverityViolation([]TestResult{r}, true) {
		t.Errorf("seq scan warning should fail under --strict in plan choice mode: %+v", r.PlanWarnings)
	}
	if hasSeverityViolation([]TestResult{r}, false) {
		t.Error("warning must not fail without --strict")
	}
}

func TestConsolePlanChoiceCriticalWithoutPlanChanged(t *testing.T) {
	r := planChoiceFailure(PlanRegression{Severity: "critical", Table: "orders", OldScan: "Index Scan", NewScan: "Seq Scan"})
	r.PlanChanged = false
	var buf bytes.Buffer
	(&ConsoleFormatter{}).printCostFailure(r, &buf)
	if !strings.Contains(buf.String(), "orders") {
		t.Errorf("regressions must print even when PlanChanged is false: %q", buf.String())
	}
}

func TestConsoleSuggestionsPlanChoice(t *testing.T) {
	f := &ConsoleFormatter{results: []TestResult{planChoiceFailure()}}
	var buf bytes.Buffer
	f.printSuggestions(&TestSummary{Failed: 1}, &buf)
	if !strings.Contains(buf.String(), "regresql baseline") || strings.Contains(buf.String(), "regresql update") {
		t.Errorf("got %q", buf.String())
	}
	f = &ConsoleFormatter{results: []TestResult{{Type: "cost", Status: "failed"}}}
	buf.Reset()
	f.printSuggestions(&TestSummary{Failed: 1}, &buf)
	if !strings.Contains(buf.String(), "regresql update <query-name>") {
		t.Errorf("legacy suggestion changed: %q", buf.String())
	}
}

func TestConsoleUninjectedBaselinesNotice(t *testing.T) {
	f := &ConsoleFormatter{}
	s := NewTestSummary()
	s.AddResult(TestResult{Type: "cost", Status: "passed", PlanChoice: true, BaselineUninjected: true})
	s.AddResult(TestResult{Type: "cost", Status: "passed", PlanChoice: true, BaselineUninjected: true})
	var buf bytes.Buffer
	if err := f.Finish(s, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "recorded without injected stats") != 1 || !strings.Contains(buf.String(), "2 baseline(s)") {
		t.Errorf("expected one aggregated notice: %q", buf.String())
	}
}
