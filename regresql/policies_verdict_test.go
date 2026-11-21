package regresql

import (
	"bytes"
	"strings"
	"testing"
)

func seqScanRegression() PlanRegression {
	return PlanRegression{
		Type:            IndexToSeqScan,
		Table:           "orders",
		OldScan:         "Index Scan using orders_pkey",
		NewScan:         "Seq Scan",
		Severity:        "critical",
		Message:         "Table 'orders' changed from Index Scan to Seq Scan",
		Recommendations: []string{"CREATE INDEX ON orders (customer_id);"},
	}
}

func packPolicies(t *testing.T, name string) *PoliciesConfig {
	t.Helper()
	return loadPackConfig(t, name).Policies
}

func TestPoliciesDecideVerdict(t *testing.T) {
	tests := []struct {
		pack         string
		wantStatus   string
		wantSeverity string
	}{
		{"saas", "passed", "warning"},
		{"fintech", "failed", "error"},
		{"", "failed", "critical"},
	}
	for _, tt := range tests {
		name := tt.pack
		if name == "" {
			name = "no pack"
		}
		t.Run(name, func(t *testing.T) {
			var policies *PoliciesConfig
			if tt.pack != "" {
				policies = packPolicies(t, tt.pack)
			}
			r := TestResult{Name: "q.cost", Type: "cost"}
			r.PlanRegressions = []PlanRegression{seqScanRegression()}

			ok, _ := applyPoliciesThenGate(&r, policies, verdictInput{
				Mode: ModeBuffers, BranchOk: true,
			})

			if got := r.PlanRegressions[0].Severity; got != tt.wantSeverity {
				t.Errorf("severity = %q, want %q", got, tt.wantSeverity)
			}
			if status, _ := bufferResult("q.cost", ok, true, &r, 0); status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}
			wantDecisions := 0
			if tt.pack != "" {
				wantDecisions = 1
			}
			if got := len(r.PolicyApplied); got != wantDecisions {
				t.Errorf("policy decisions = %d, want %d (%+v)", got, wantDecisions, r.PolicyApplied)
			}
		})
	}
}

func TestFintechErrorRegressionShowsRecommendations(t *testing.T) {
	r := TestResult{Name: "q.cost", Type: "cost", Status: "failed"}
	r.PlanRegressions = []PlanRegression{seqScanRegression()}

	if ok, _ := applyPoliciesThenGate(&r, packPolicies(t, "fintech"), verdictInput{
		Mode: ModeBuffers, BranchOk: true,
	}); ok {
		t.Fatal("an error regression must fail the verdict")
	}

	var buf bytes.Buffer
	(&ConsoleFormatter{}).printCostFailure(r, &buf)
	if !strings.Contains(buf.String(), "Recommendations:") {
		t.Errorf("error regression must keep its recommendations: %q", buf.String())
	}
}
