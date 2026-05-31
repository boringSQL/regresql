package regresql

import (
	"encoding/json"
	"testing"
)

// SumTuplesProcessed must sum actual rows × loops across the whole tree, so a
// nested-loop inner side scanned once per outer row is counted in full.
func TestSumTuplesProcessed(t *testing.T) {
	raw := `{"Plan":{
		"Node Type":"Nested Loop","Actual Rows":10,"Actual Loops":1,
		"Plans":[
			{"Node Type":"Seq Scan","Actual Rows":5,"Actual Loops":1},
			{"Node Type":"Index Scan","Actual Rows":2,"Actual Loops":5}
		]}}`

	var out ExplainOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// 10*1 (root) + 5*1 (seq) + 2*5 (index, 5 loops) = 25
	got := SumTuplesProcessed(&out.Plan)
	if got != 25 {
		t.Errorf("SumTuplesProcessed = %v, want 25", got)
	}
}

// A node with no ANALYZE data (Actual Loops absent → 0) is treated as 1 loop so
// the row count still contributes rather than vanishing.
func TestSumTuplesProcessed_MissingLoops(t *testing.T) {
	raw := `{"Plan":{"Node Type":"Result","Actual Rows":7}}`

	var out ExplainOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := SumTuplesProcessed(&out.Plan); got != 7 {
		t.Errorf("SumTuplesProcessed = %v, want 7", got)
	}
}

func TestCompareTuples(t *testing.T) {
	cases := []struct {
		name             string
		actual, baseline float64
		wantOk           bool
	}{
		{"zero baseline is no signal", 1e6, 0, true},
		{"identical", 1000, 1000, true},
		{"within threshold", 1050, 1000, true},
		{"grew past threshold", 1300, 1000, false},
		{"fewer tuples is fine", 500, 1000, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, _ := CompareTuples(tc.actual, tc.baseline, DefaultCostThresholdPercent)
			if ok != tc.wantOk {
				t.Errorf("CompareTuples(%v, %v) ok = %v, want %v", tc.actual, tc.baseline, ok, tc.wantOk)
			}
		})
	}
}
