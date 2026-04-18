package regresql

import "testing"

func TestApplyPolicies_EscalatesWarning(t *testing.T) {
	r := &TestResult{
		PlanWarnings: []PlanWarning{
			{Type: SeqScanDetected, Severity: "warning", Table: "widgets"},
		},
	}
	p := &PoliciesConfig{
		Severity: map[string]string{"sequential_scan_detected": "error"},
	}

	ApplyPolicies(r, p)

	if r.PlanWarnings[0].Severity != "error" {
		t.Errorf("expected warning escalated to error, got %q", r.PlanWarnings[0].Severity)
	}
	if len(r.PolicyApplied) != 1 {
		t.Fatalf("expected 1 PolicyDecision, got %d", len(r.PolicyApplied))
	}
	d := r.PolicyApplied[0]
	if d.Rule != "sequential_scan_detected" || d.FromSeverity != "warning" || d.ToSeverity != "error" || d.Table != "widgets" {
		t.Errorf("unexpected decision: %+v", d)
	}
}

func TestApplyPolicies_DowngradesCriticalSeqScan(t *testing.T) {
	r := &TestResult{
		PlanWarnings: []PlanWarning{
			{Type: SeqScanOnCriticalTable, Severity: "error", Table: "orders"},
		},
	}
	p := &PoliciesConfig{
		Severity: map[string]string{"seq_scan_critical_table": "warning"},
	}

	ApplyPolicies(r, p)

	if r.PlanWarnings[0].Severity != "warning" {
		t.Errorf("expected downgrade to warning, got %q", r.PlanWarnings[0].Severity)
	}
	if len(r.PolicyApplied) != 1 {
		t.Fatalf("expected 1 PolicyDecision, got %d", len(r.PolicyApplied))
	}
	if r.PolicyApplied[0].FromSeverity != "error" || r.PolicyApplied[0].ToSeverity != "warning" {
		t.Errorf("unexpected From/To: %+v", r.PolicyApplied[0])
	}
}

func TestApplyPolicies_RegressionRemapped(t *testing.T) {
	r := &TestResult{
		PlanRegressions: []PlanRegression{
			{Type: IndexToSeqScan, Severity: "critical", Table: "orders"},
		},
	}
	p := &PoliciesConfig{
		Severity: map[string]string{"index_to_seqscan": "error"},
	}

	ApplyPolicies(r, p)

	if r.PlanRegressions[0].Severity != "error" {
		t.Errorf("expected regression severity error, got %q", r.PlanRegressions[0].Severity)
	}
	if len(r.PolicyApplied) != 1 || r.PolicyApplied[0].Rule != "index_to_seqscan" {
		t.Errorf("expected decision for regression, got %+v", r.PolicyApplied)
	}
}

func TestApplyPolicies_NoConfigNoOp(t *testing.T) {
	r := &TestResult{
		PlanWarnings: []PlanWarning{
			{Type: SeqScanDetected, Severity: "warning"},
		},
	}

	ApplyPolicies(r, nil)

	if r.PlanWarnings[0].Severity != "warning" {
		t.Errorf("nil config must not mutate severity")
	}
	if len(r.PolicyApplied) != 0 {
		t.Errorf("nil config must not record decisions")
	}
}

func TestApplyPolicies_EmptySeverityMapNoOp(t *testing.T) {
	r := &TestResult{
		PlanWarnings: []PlanWarning{
			{Type: SeqScanDetected, Severity: "warning"},
		},
	}

	ApplyPolicies(r, &PoliciesConfig{})

	if len(r.PolicyApplied) != 0 {
		t.Errorf("empty severity map must not record decisions")
	}
}

func TestApplyPolicies_SameSeverityNoOp(t *testing.T) {
	r := &TestResult{
		PlanWarnings: []PlanWarning{
			{Type: SeqScanDetected, Severity: "warning"},
		},
	}
	p := &PoliciesConfig{
		Severity: map[string]string{"sequential_scan_detected": "warning"},
	}

	ApplyPolicies(r, p)

	if len(r.PolicyApplied) != 0 {
		t.Errorf("same-severity mapping must not record a decision")
	}
}

func TestApplyPolicies_ReasonPulledThrough(t *testing.T) {
	r := &TestResult{
		PlanWarnings: []PlanWarning{
			{Type: SeqScanOnCriticalTable, Severity: "error", Table: "orders"},
		},
	}
	p := &PoliciesConfig{
		Severity: map[string]string{"seq_scan_critical_table": "warning"},
		Reasons:  map[string]string{"seq_scan_critical_table": "compliance"},
	}

	ApplyPolicies(r, p)

	if len(r.PolicyApplied) != 1 || r.PolicyApplied[0].Reason != "compliance" {
		t.Errorf("expected reason 'compliance', got %+v", r.PolicyApplied)
	}
}
