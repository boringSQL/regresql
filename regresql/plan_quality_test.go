package regresql

import "testing"

func buildSigWithSeqScans(tables ...string) *PlanSignature {
	sig := &PlanSignature{
		Relations:  make(map[string]ScanInfo),
		HasSeqScan: len(tables) > 0,
	}
	for _, t := range tables {
		sig.Relations[t] = ScanInfo{ScanType: "Seq Scan"}
	}
	return sig
}

func findWarning(warnings []PlanWarning, wType WarningType, table string) *PlanWarning {
	for i := range warnings {
		if warnings[i].Type == wType && warnings[i].Table == table {
			return &warnings[i]
		}
	}
	return nil
}

// Cost large enough to clear both trivial-cost thresholds.
var nonTrivialCost = PlanCostInfo{TotalCost: 1000.0, TotalBuffers: 1000}

func TestDetectPlanQualityIssues_CriticalTableEmitsError(t *testing.T) {
	sig := buildSigWithSeqScans("orders")
	warnings := DetectPlanQualityIssues(
		sig, RegressQLOptions{},
		nil, []string{"orders"},
		nonTrivialCost,
	)

	w := findWarning(warnings, SeqScanOnCriticalTable, "orders")
	if w == nil {
		t.Fatalf("expected SeqScanOnCriticalTable warning for 'orders', got %+v", warnings)
	}
	if w.Severity != "error" {
		t.Errorf("expected Severity=error, got %q", w.Severity)
	}
	if generic := findWarning(warnings, SeqScanDetected, "orders"); generic != nil {
		t.Errorf("expected no duplicate generic SeqScanDetected for the same critical table, got %+v", generic)
	}
}

func TestDetectPlanQualityIssues_CriticalBeatsIgnored(t *testing.T) {
	sig := buildSigWithSeqScans("orders")
	warnings := DetectPlanQualityIssues(
		sig, RegressQLOptions{},
		[]string{"orders"},     // same table also in ignore list
		[]string{"orders"},
		nonTrivialCost,
	)

	w := findWarning(warnings, SeqScanOnCriticalTable, "orders")
	if w == nil {
		t.Fatalf("critical should win over ignore; got %+v", warnings)
	}
	if w.Severity != "error" {
		t.Errorf("expected Severity=error, got %q", w.Severity)
	}
}

func TestDetectPlanQualityIssues_CriticalBypassesTrivialCost(t *testing.T) {
	sig := buildSigWithSeqScans("orders")
	trivial := PlanCostInfo{TotalCost: 1.0, TotalBuffers: 1}
	warnings := DetectPlanQualityIssues(
		sig, RegressQLOptions{},
		nil, []string{"orders"},
		trivial,
	)
	if findWarning(warnings, SeqScanOnCriticalTable, "orders") == nil {
		t.Fatalf("critical table should bypass trivial-cost filter; got %+v", warnings)
	}
}

func TestDetectPlanQualityIssues_NonCriticalUnchanged(t *testing.T) {
	sig := buildSigWithSeqScans("widgets")
	warnings := DetectPlanQualityIssues(
		sig, RegressQLOptions{},
		nil, []string{"orders"},
		nonTrivialCost,
	)

	w := findWarning(warnings, SeqScanDetected, "widgets")
	if w == nil {
		t.Fatalf("expected SeqScanDetected for non-critical 'widgets', got %+v", warnings)
	}
	if w.Severity != "warning" {
		t.Errorf("expected Severity=warning, got %q", w.Severity)
	}
}

func TestDetectPlanQualityIssues_NoCriticalConfigNoRegression(t *testing.T) {
	sig := buildSigWithSeqScans("widgets")
	warnings := DetectPlanQualityIssues(
		sig, RegressQLOptions{},
		nil, nil,
		nonTrivialCost,
	)

	w := findWarning(warnings, SeqScanDetected, "widgets")
	if w == nil || w.Severity != "warning" {
		t.Fatalf("expected unchanged seq-scan warning, got %+v", warnings)
	}
}

func TestHasSeverityViolation(t *testing.T) {
	results := []TestResult{
		{PlanWarnings: []PlanWarning{{Severity: "error"}}},
	}
	if !hasSeverityViolation(results, false) {
		t.Errorf("error-severity warning must trigger violation")
	}

	warnOnly := []TestResult{
		{PlanWarnings: []PlanWarning{{Severity: "warning"}}},
	}
	if hasSeverityViolation(warnOnly, false) {
		t.Errorf("warning without --strict must not trigger violation")
	}
	if !hasSeverityViolation(warnOnly, true) {
		t.Errorf("warning under --strict must trigger violation")
	}

	regressions := []TestResult{
		{PlanRegressions: []PlanRegression{{Severity: "error"}}},
	}
	if !hasSeverityViolation(regressions, false) {
		t.Errorf("error-severity regression must trigger violation")
	}

	clean := []TestResult{
		{PlanWarnings: []PlanWarning{{Severity: "info"}}},
	}
	if hasSeverityViolation(clean, true) {
		t.Errorf("info severity should never trigger violation")
	}
}
