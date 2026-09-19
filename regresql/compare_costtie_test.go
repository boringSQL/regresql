package regresql

import (
	"strings"
	"testing"
)

// Excluded and annotated ties are opposite facts about whether a query was
// tested; the headline must not render both as a bare "N cost-tie".
func TestHeadlineDistinguishesExcludedFromAnnotatedTies(t *testing.T) {
	excluded := (&Scoreboard{
		Comparisons: make([]QueryComparison, 67),
		CostTie:     make([]AdmitResult, 46),
	}).totals().line()
	if !strings.Contains(excluded, "67 queries") || !strings.Contains(excluded, "46 cost-tie (excluded)") {
		t.Errorf("excluded ties not reported as excluded: %s", excluded)
	}

	annotated := (&Scoreboard{
		Comparisons:      make([]QueryComparison, 113),
		CostTieAnnotated: 46,
	}).totals().line()
	if !strings.Contains(annotated, "113 queries") ||
		!strings.Contains(annotated, "46 cost-tie (compared, annotated)") {
		t.Errorf("annotated ties not reported as compared: %s", annotated)
	}
	if strings.Contains(annotated, "(excluded)") {
		t.Errorf("annotated ties must not claim exclusion: %s", annotated)
	}

	// Neither counter may appear when there are no ties at all.
	clean := (&Scoreboard{Comparisons: make([]QueryComparison, 12)}).totals().line()
	if strings.Contains(clean, "cost-tie") {
		t.Errorf("no ties, but headline mentions them: %s", clean)
	}
}

// Both preflights feed whichever map --self-control selects: start from nil,
// keep the first note per key, ignore empty inputs.
func TestMergeNotes(t *testing.T) {
	got := mergeNotes(nil, map[string]string{"a": "tie"})
	if got["a"] != "tie" {
		t.Fatalf("merge into nil map lost the entry: %v", got)
	}
	got = mergeNotes(got, map[string]string{"a": "unstable", "b": "unstable"})
	if got["a"] != "tie" {
		t.Errorf("second source overwrote an existing note: %q", got["a"])
	}
	if got["b"] != "unstable" {
		t.Errorf("second source did not add a new key: %v", got)
	}
	if mergeNotes(nil, nil) != nil {
		t.Error("merging nothing into nil should stay nil, not allocate")
	}
	if n := len(mergeNotes(got, map[string]string{})); n != 2 {
		t.Errorf("empty source changed the map: %d entries", n)
	}
}
