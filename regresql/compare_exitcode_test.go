package regresql

import "testing"

// the gated/advisory split has to hold in the exit code too, or a shape-only
// finding fails the run while every headline counter reads zero
func TestExitCodeIgnoresAdvisoryTiers(t *testing.T) {
	cases := []struct {
		name string
		sev  Severity
		want int
	}{
		{"equal passes", SevEqual, 0},
		{"plan shape is advisory -- does not fail the run", SevShape, 0},
		{"q-error is advisory -- does not fail the run", SevEstimation, 0},
		{"buffers/spill are gated", SevPerf, 1},
		{"incomplete is gated", SevIncomplete, 1},
		{"correctness is gated", SevCorrectness, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &Scoreboard{Comparisons: []QueryComparison{{Severity: c.sev}}}
			if got := b.exitCode(); got != c.want {
				t.Errorf("exitCode() = %d, want %d for %v", got, c.want, c.sev)
			}
		})
	}
}
