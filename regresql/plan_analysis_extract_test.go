package regresql

import (
	"encoding/json"
	"slices"
	"testing"
)

// A Hash Join whose Join Type qualifier is "Right Anti" must land in JoinModes
// alongside its node type in JoinTypes. This is the typed-extractor path used
// by the live capture flow (ExtractPlanSignatureFromNode).
func TestExtractFromTypedNode_CapturesJoinMode(t *testing.T) {
	node := &PlanNode{
		NodeType: "Hash Join",
		JoinType: "Right Anti",
		Plans: []PlanNode{
			{NodeType: "Seq Scan", RelationName: "orders"},
		},
	}

	sig := ExtractPlanSignatureFromNode(node)

	if !slices.Equal(sig.JoinTypes, []string{"Hash Join"}) {
		t.Errorf("JoinTypes = %v, want [Hash Join]", sig.JoinTypes)
	}
	if !slices.Equal(sig.JoinModes, []string{"Right Anti"}) {
		t.Errorf("JoinModes = %v, want [Right Anti]", sig.JoinModes)
	}
}

// Only "Partial" and "Finalize" are interesting; the default "Simple" mode (and
// the empty string for nodes that never report one) must be dropped so two
// ordinary aggregates compare equal.
func TestExtractFromTypedNode_PartialModeFilter(t *testing.T) {
	node := &PlanNode{
		NodeType: "Finalize Aggregate",
		// The root carries the Finalize half of an eager-aggregation split...
		PartialMode: "Finalize",
		Plans: []PlanNode{
			{
				NodeType: "Gather",
				Plans: []PlanNode{
					// ...and the Partial half sits below the Gather.
					{NodeType: "Partial Aggregate", PartialMode: "Partial"},
				},
			},
			// A plain aggregate reporting "Simple" must be ignored entirely.
			{NodeType: "Aggregate", PartialMode: "Simple"},
		},
	}

	sig := ExtractPlanSignatureFromNode(node)

	want := []string{"Finalize", "Partial"}
	if !slices.Equal(sig.PartialModes, want) {
		t.Errorf("PartialModes = %v, want %v", sig.PartialModes, want)
	}
}

// The untyped extractor (ExtractPlanSignature, kept for backwards compatibility
// with map-shaped plans) must read the very same JSON keys — "Join Type" and
// "Partial Mode" — so baselines captured through either path stay comparable.
func TestExtractFromUntypedNode_CapturesJoinModeAndPartial(t *testing.T) {
	raw := `{
		"Plan": {
			"Node Type": "Finalize Aggregate",
			"Partial Mode": "Finalize",
			"Plans": [
				{
					"Node Type": "Hash Join",
					"Join Type": "Semi",
					"Plans": [
						{"Node Type": "Partial Aggregate", "Partial Mode": "Partial"}
					]
				}
			]
		}
	}`

	var explainPlan map[string]any
	if err := json.Unmarshal([]byte(raw), &explainPlan); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	sig, err := ExtractPlanSignature(explainPlan)
	if err != nil {
		t.Fatalf("ExtractPlanSignature: %v", err)
	}

	if !slices.Equal(sig.JoinModes, []string{"Semi"}) {
		t.Errorf("JoinModes = %v, want [Semi]", sig.JoinModes)
	}
	if !slices.Equal(sig.PartialModes, []string{"Finalize", "Partial"}) {
		t.Errorf("PartialModes = %v, want [Finalize Partial]", sig.PartialModes)
	}
}

// HasPlanChanged is the coarse "did anything move" gate. It must now treat a
// join-qualifier flip and a partial-mode flip as changes even when node types,
// relations, and every other axis are byte-for-byte identical.
func TestHasPlanChanged_DetectsModeFlips(t *testing.T) {
	base := func() *PlanSignature {
		return &PlanSignature{
			Relations:    make(map[string]ScanInfo),
			JoinTypes:    []string{"Hash Join"},
			JoinModes:    []string{"Inner"},
			PartialModes: []string{"Partial"},
		}
	}

	t.Run("identical signatures are unchanged", func(t *testing.T) {
		if HasPlanChanged(base(), base()) {
			t.Error("identical signatures must report no change")
		}
	})

	t.Run("join qualifier flip is a change", func(t *testing.T) {
		current := base()
		current.JoinModes = []string{"Right Anti"}
		if !HasPlanChanged(base(), current) {
			t.Error("Inner -> Right Anti must report a change")
		}
	})

	t.Run("partial mode flip is a change", func(t *testing.T) {
		current := base()
		current.PartialModes = nil
		if !HasPlanChanged(base(), current) {
			t.Error("losing a Partial aggregate must report a change")
		}
	})
}
