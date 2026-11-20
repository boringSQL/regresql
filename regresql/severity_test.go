package regresql

import "testing"

func TestSeverityRankOrder(t *testing.T) {
	order := []Severity{SevInfo, SevWarning, SevError, SevCritical}
	for i := 1; i < len(order); i++ {
		if order[i-1].Rank() >= order[i].Rank() {
			t.Errorf("%s must rank below %s", order[i-1], order[i])
		}
	}
	if got := Severity("blocker").Rank(); got != -1 {
		t.Errorf("unknown severity rank = %d, want -1", got)
	}
}

func TestSeverityAtLeast(t *testing.T) {
	tests := []struct {
		s     string
		floor Severity
		want  bool
	}{
		{"critical", SevCritical, true},
		{"error", SevCritical, false},
		{"error", SevError, true},
		{"warning", SevWarning, true},
		{"error", SevWarning, true},
		{"info", SevWarning, false},
		{"info", SevInfo, true},
		{"", SevInfo, false},
		{"blocker", SevInfo, false},
		{"error", Severity("blocker"), false},
		{"blocker", Severity("blocker"), false},
		{"", Severity(""), false},
	}
	for _, tt := range tests {
		if got := SeverityAtLeast(tt.s, tt.floor); got != tt.want {
			t.Errorf("SeverityAtLeast(%q, %s) = %v, want %v", tt.s, tt.floor, got, tt.want)
		}
	}
}

func TestParseSeverity(t *testing.T) {
	for _, s := range []string{"info", "warning", "error", "critical"} {
		sev, err := ParseSeverity(s)
		if err != nil || string(sev) != s {
			t.Errorf("ParseSeverity(%q) = %q, %v", s, sev, err)
		}
	}
	if _, err := ParseSeverity("blocker"); err == nil {
		t.Error("ParseSeverity must reject an unknown value")
	}
}

func TestHasSeverityViolationLadder(t *testing.T) {
	tests := []struct {
		sev          string
		strict, want bool
	}{
		{"info", true, false},
		{"warning", false, false},
		{"warning", true, true},
		{"error", false, true},
		{"critical", false, true},
		{"blocker", true, false},
	}
	for _, tt := range tests {
		r := TestResult{PlanWarnings: []PlanWarning{{Severity: tt.sev}}}
		if got := hasSeverityViolation([]TestResult{r}, tt.strict); got != tt.want {
			t.Errorf("severity %q strict=%v: got %v, want %v", tt.sev, tt.strict, got, tt.want)
		}
	}
}
