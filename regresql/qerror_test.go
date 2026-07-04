package regresql

import (
	"encoding/json"
	"testing"
)

// WorstQError must pick the single worst node across the whole tree — a deep
// child that is 500x off outranks a root that is only 2x off. The label carries
// the relation name so the verdict says where the estimate broke.
func TestWorstQError(t *testing.T) {
	raw := `{"Plan":{
		"Node Type":"Nested Loop","Plan Rows":10,"Actual Rows":20,"Actual Loops":1,
		"Plans":[
			{"Node Type":"Seq Scan","Relation Name":"orders","Plan Rows":1,"Actual Rows":500,"Actual Loops":1},
			{"Node Type":"Index Scan","Relation Name":"users","Plan Rows":5,"Actual Rows":5,"Actual Loops":1}
		]}}`

	var out ExplainOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	worst := WorstQError(&out.Plan)
	if worst == nil {
		t.Fatal("WorstQError = nil, want the orders Seq Scan")
	}
	if worst.QError != 500 {
		t.Errorf("QError = %v, want 500", worst.QError)
	}
	if got := qErrorNodeLabel(worst); got != "Seq Scan on orders" {
		t.Errorf("label = %q, want %q", got, "Seq Scan on orders")
	}
}

// q-error is symmetric: a 100x overestimate is as bad as a 100x underestimate.
// Both sides clamp to >= 1, so actual = 0 yields est/1 instead of dividing by zero.
func TestWorstQError_ClampsAndSymmetric(t *testing.T) {
	raw := `{"Plan":{"Node Type":"Seq Scan","Plan Rows":1000,"Actual Rows":0,"Actual Loops":1}}`

	var out ExplainOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	worst := WorstQError(&out.Plan)
	if worst == nil || worst.QError != 1000 {
		t.Fatalf("QError = %+v, want 1000 (overestimate against clamped actual)", worst)
	}
}

// Nodes the executor never ran (Actual Loops = 0, e.g. a pruned subplan or the
// inner side behind a one-time false filter) carry no measured truth — their
// wild estimates must not fabricate a q-error.
func TestWorstQError_SkipsNeverExecuted(t *testing.T) {
	raw := `{"Plan":{
		"Node Type":"Append","Plan Rows":10,"Actual Rows":10,"Actual Loops":1,
		"Plans":[
			{"Node Type":"Seq Scan","Relation Name":"pruned","Plan Rows":1000000,"Actual Rows":0,"Actual Loops":0}
		]}}`

	var out ExplainOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	worst := WorstQError(&out.Plan)
	if worst == nil {
		t.Fatal("WorstQError = nil, want the executed Append root")
	}
	if worst.NodeType != "Append" || worst.QError != 1 {
		t.Errorf("worst = %s %vx, want Append 1x (pruned child ignored)", worst.NodeType, worst.QError)
	}
}

// The gate needs BOTH conditions: worse than baseline by the ratio AND past the
// absolute floor. Either alone is noise — small baselines double trivially, and
// a chronically bad estimate that didn't change is not a regression.
func TestIsQErrorRegression(t *testing.T) {
	const ratio, floor = 2.0, 10.0

	cases := []struct {
		name             string
		actual, baseline float64
		want             bool
	}{
		{"both ratio and floor exceeded", 340, 2, true},
		{"ratio hit but under floor", 8, 2, false},
		{"floor hit but chronic (ratio not hit)", 120, 100, false},
		{"missing baseline is no signal", 1000, 0, false},
		{"unchanged", 50, 50, false},
		{"improved", 5, 50, false},
		{"exactly at both thresholds", 10, 5, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsQErrorRegression(tc.actual, tc.baseline, ratio, floor); got != tc.want {
				t.Errorf("IsQErrorRegression(%v, %v) = %v, want %v", tc.actual, tc.baseline, got, tc.want)
			}
		})
	}
}

// Rows a node examined but discarded are real CPU work: a Seq Scan that reads
// 5000 rows per loop and emits 10 must count as 5010 per loop, not 10 — the
// exact class of regression the emitted-only sum was blind to.
func TestSumTuplesProcessed_CountsRemovedRows(t *testing.T) {
	raw := `{"Plan":{
		"Node Type":"Seq Scan","Actual Rows":10,"Actual Loops":2,
		"Rows Removed by Filter":4000,
		"Rows Removed by Join Filter":800,
		"Rows Removed by Index Recheck":200}}`

	var out ExplainOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// (10 + 4000 + 800 + 200) * 2 loops = 10020
	if got := SumTuplesProcessed(&out.Plan); got != 10020 {
		t.Errorf("SumTuplesProcessed = %v, want 10020", got)
	}
}
