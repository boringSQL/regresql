package regresql

import (
	"encoding/json"
	"testing"
)

// newSig builds a minimal PlanSignature for HasPlanChanged tests.
func newSig(nodeTypes []string, subplansRemoved int) *PlanSignature {
	return &PlanSignature{
		NodeTypes:       nodeTypes,
		Relations:       map[string]ScanInfo{},
		SubplansRemoved: subplansRemoved,
	}
}

// guards "Subplans Removed" JSON key wiring on both extract paths
func TestSubplansRemoved_Extraction(t *testing.T) {
	const planJSON = `{"Node Type":"Append","Subplans Removed":3,"Plans":[
		{"Node Type":"Seq Scan","Relation Name":"sales_p2"}]}`

	// typed path
	var node PlanNode
	if err := json.Unmarshal([]byte(planJSON), &node); err != nil {
		t.Fatal(err)
	}
	if got := ExtractPlanSignatureFromNode(&node).SubplansRemoved; got != 3 {
		t.Errorf("typed path SubplansRemoved = %d, want 3", got)
	}

	// map path
	var m map[string]any
	if err := json.Unmarshal([]byte(`{"Plan":`+planJSON+`}`), &m); err != nil {
		t.Fatal(err)
	}
	sig, err := ExtractPlanSignature(m)
	if err != nil {
		t.Fatal(err)
	}
	if sig.SubplansRemoved != 3 {
		t.Errorf("map path SubplansRemoved = %d, want 3", sig.SubplansRemoved)
	}
}

func TestHasPlanChanged_NodeMultiset(t *testing.T) {
	cases := []struct {
		name    string
		a, b    *PlanSignature
		changed bool
	}{
		{"identical",
			newSig([]string{"Aggregate", "Hash Join", "Seq Scan"}, 0),
			newSig([]string{"Aggregate", "Hash Join", "Seq Scan"}, 0), false},
		{"append_vs_mergeappend",
			newSig([]string{"Append", "Seq Scan", "Seq Scan"}, 0),
			newSig([]string{"Merge Append", "Seq Scan", "Seq Scan"}, 0), true},
		{"partitionwise_agg_node_count", // 2 per-partition Aggregates vs 1
			newSig([]string{"Append", "Aggregate", "Aggregate"}, 0),
			newSig([]string{"Aggregate", "Append"}, 0), true},
		{"subplans_removed_runtime_pruning",
			newSig([]string{"Append", "Seq Scan"}, 0),
			newSig([]string{"Append", "Seq Scan"}, 4), true},
		{"incidental_node_tolerated", // Materialize appears on a cost-tie replan
			newSig([]string{"Nested Loop", "Seq Scan", "Index Scan"}, 0),
			newSig([]string{"Nested Loop", "Seq Scan", "Materialize", "Index Scan"}, 0), false},
		{"node_multiset_reorder_invariant",
			newSig([]string{"Aggregate", "Append", "Seq Scan"}, 0),
			newSig([]string{"Seq Scan", "Aggregate", "Append"}, 0), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasPlanChanged(c.a, c.b); got != c.changed {
				t.Errorf("HasPlanChanged = %v, want %v", got, c.changed)
			}
		})
	}
}
