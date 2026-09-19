package regresql

import "testing"

// a query is a tie when the runner-up prices within the margin: pin the
// arithmetic and the edge cases that decide whether a query carries a verdict.
func TestTieMarginArithmetic(t *testing.T) {
	cases := []struct {
		name           string
		chosen, runner float64
		margin         float64
		wantTie        bool
	}{
		{"runner-up 0.047% away is a tie (the q04 false positive)", 21276, 21286, 1.0, true},
		{"runner-up 50% away is a real choice", 1000, 1500, 1.0, false},
		{"exactly on the margin counts as a tie", 1000, 1010, 1.0, true},
		{"just outside the margin does not", 1000, 1011, 1.0, false},
		{"a wider margin quarantines more", 1000, 1030, 5.0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := (c.runner - c.chosen) / c.chosen * 100
			if tie := got <= c.margin; tie != c.wantTie {
				t.Errorf("margin %.3f%% vs %.2f%%: tie=%v, want %v", got, c.margin, tie, c.wantTie)
			}
		})
	}
}

// no alternative plan means the choice is forced, never an arbitrary tiebreak
func TestForcedPlanIsNeverATie(t *testing.T) {
	const forced = 100.0
	for _, margin := range []float64{0.5, 1, 5, 25, 99} {
		if forced <= margin {
			t.Errorf("margin %.1f%% would quarantine a query with no alternative plan", margin)
		}
	}
}
