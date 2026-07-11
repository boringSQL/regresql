package regresql

import (
	"bytes"
	"strings"
	"testing"
)

func fingerprint(n *PlanNode) string {
	var b strings.Builder
	planFingerprint(n, &b)
	return b.String()
}

// The whole point of the fingerprint: two plans with the SAME nodes in a
// DIFFERENT join order must fingerprint differently — that's how a cost-tie
// reorder (30c: same node multiset, swinging buffers) gets caught.
func TestPlanFingerprint_OrderSensitive(t *testing.T) {
	ab := &PlanNode{NodeType: "Nested Loop", Plans: []PlanNode{
		{NodeType: "Seq Scan", RelationName: "a"},
		{NodeType: "Index Scan", RelationName: "b"},
	}}
	ba := &PlanNode{NodeType: "Nested Loop", Plans: []PlanNode{
		{NodeType: "Index Scan", RelationName: "b"},
		{NodeType: "Seq Scan", RelationName: "a"},
	}}

	if fingerprint(ab) == fingerprint(ba) {
		t.Errorf("reordered joins fingerprinted the same:\n  %s", fingerprint(ab))
	}
}

// The same tree always fingerprints identically (a stable plan isn't a cost tie).
func TestPlanFingerprint_Stable(t *testing.T) {
	tree := func() *PlanNode {
		return &PlanNode{NodeType: "Hash Join", Plans: []PlanNode{
			{NodeType: "Seq Scan", RelationName: "orders"},
			{NodeType: "Hash", Plans: []PlanNode{{NodeType: "Seq Scan", RelationName: "customers"}}},
		}}
	}
	if fingerprint(tree()) != fingerprint(tree()) {
		t.Error("identical plans fingerprinted differently")
	}
}

// A shape change (different node type) and a relation swap both move the
// fingerprint — it captures node type, relation, and tree structure.
func TestPlanFingerprint_ShapeAndRelation(t *testing.T) {
	base := &PlanNode{NodeType: "Hash Join", Plans: []PlanNode{{NodeType: "Seq Scan", RelationName: "t"}}}

	shape := &PlanNode{NodeType: "Merge Join", Plans: []PlanNode{{NodeType: "Seq Scan", RelationName: "t"}}}
	if fingerprint(base) == fingerprint(shape) {
		t.Error("node-type change not reflected in fingerprint")
	}

	rel := &PlanNode{NodeType: "Hash Join", Plans: []PlanNode{{NodeType: "Seq Scan", RelationName: "u"}}}
	if fingerprint(base) == fingerprint(rel) {
		t.Error("relation change not reflected in fingerprint")
	}
}

// bindingKey keeps distinct (query, binding) pairs distinct.
func TestBindingKey(t *testing.T) {
	if bindingKey("q", "a") == bindingKey("q", "b") {
		t.Error("different bindings collided")
	}
	if bindingKey("qa", "") == bindingKey("q", "a") {
		t.Error("key concatenation is ambiguous")
	}
}

// Cost-tie exclusions surface in the scoreboard (the ⊘ TIE rows + the tally) so
// the reader sees what was filtered out, not a silently shrunk corpus.
func TestRenderScoreboard_CostTie(t *testing.T) {
	board := &Scoreboard{
		Base:    EngineInfo{Version: "18.4"},
		Target:  EngineInfo{Version: "19"},
		CostTie: []AdmitResult{{Name: "30c", Reason: "plan unstable across re-ANALYZE (cost tie)"}},
	}

	var console bytes.Buffer
	board.renderConsole(&console)
	for _, want := range []string{"TIE", "30c", "1 cost-tie"} {
		if !strings.Contains(console.String(), want) {
			t.Errorf("console missing %q:\n%s", want, console.String())
		}
	}

	var md bytes.Buffer
	board.renderMarkdown(&md)
	if !strings.Contains(md.String(), "cost-tie") || !strings.Contains(md.String(), "30c") {
		t.Errorf("markdown missing cost-tie row:\n%s", md.String())
	}
}
