package regresql

import (
	"reflect"
	"testing"
)

func scanNode(nodeType, schema, rel, alias string) PlanNode {
	return PlanNode{NodeType: nodeType, Schema: schema, RelationName: rel, Alias: alias}
}

func TestGenericScanFlips(t *testing.T) {
	big := map[string]int64{"public.orders": 5000, "public.users": 5000}
	orders := genericFlip{Schema: "public", Relation: "orders"}

	tests := []struct {
		name     string
		custom   PlanNode
		generic  PlanNode
		relpages map[string]int64
		want     []genericFlip
	}{
		{"index to seq", scanNode("Index Scan", "public", "orders", "orders"), scanNode("Seq Scan", "public", "orders", "orders"), big, []genericFlip{orders}},
		{"index only to seq", scanNode("Index Only Scan", "public", "orders", "orders"), scanNode("Seq Scan", "public", "orders", "orders"), big, []genericFlip{orders}},
		{"bitmap heap to seq", scanNode("Bitmap Heap Scan", "public", "orders", "orders"), scanNode("Seq Scan", "public", "orders", "orders"), big, []genericFlip{orders}},
		{"seq stays seq", scanNode("Seq Scan", "public", "orders", "orders"), scanNode("Seq Scan", "public", "orders", "orders"), big, nil},
		{"index stays index", scanNode("Index Scan", "public", "orders", "orders"), scanNode("Index Scan", "public", "orders", "orders"), big, nil},
		{"below page floor", scanNode("Index Scan", "public", "orders", "orders"), scanNode("Seq Scan", "public", "orders", "orders"), map[string]int64{"public.orders": 999}, nil},
		{"relpages missing", scanNode("Index Scan", "public", "orders", "orders"), scanNode("Seq Scan", "public", "orders", "orders"), nil, nil},
		{"other schema not matched", scanNode("Index Scan", "public", "orders", "orders"), scanNode("Seq Scan", "archive", "orders", "orders"), big, nil},
		{
			"nested parallel seq scan counts",
			scanNode("Index Scan", "public", "orders", "orders"),
			PlanNode{NodeType: "Gather", Plans: []PlanNode{{NodeType: "Hash Join", Plans: []PlanNode{
				{NodeType: "Seq Scan", ParallelAware: true, Schema: "public", RelationName: "orders", Alias: "orders"},
			}}}},
			big, []genericFlip{orders},
		},
		{"floor is inclusive", scanNode("Index Scan", "public", "orders", "orders"), scanNode("Seq Scan", "public", "orders", "orders"), map[string]int64{"public.orders": 1000}, []genericFlip{orders}},
		{"seq to index is not a flip", scanNode("Seq Scan", "public", "orders", "orders"), scanNode("Index Scan", "public", "orders", "orders"), big, nil},
		{
			"update wrapping scan",
			PlanNode{NodeType: "ModifyTable", Schema: "public", RelationName: "orders", Alias: "orders", Plans: []PlanNode{scanNode("Index Scan", "public", "orders", "orders")}},
			PlanNode{NodeType: "ModifyTable", Schema: "public", RelationName: "orders", Alias: "orders", Plans: []PlanNode{scanNode("Seq Scan", "public", "orders", "orders")}},
			big, []genericFlip{orders},
		},
		{
			"key three times skipped",
			PlanNode{NodeType: "Append", Plans: []PlanNode{
				scanNode("Index Scan", "public", "orders", "orders"),
				scanNode("Index Scan", "public", "orders", "orders"),
				scanNode("Index Scan", "public", "orders", "orders"),
			}},
			scanNode("Seq Scan", "public", "orders", "orders"),
			big, nil,
		},
		{
			"duplicate only in generic skipped",
			scanNode("Index Scan", "public", "orders", "orders"),
			PlanNode{NodeType: "Append", Plans: []PlanNode{
				scanNode("Seq Scan", "public", "orders", "orders"),
				scanNode("Seq Scan", "public", "orders", "orders"),
			}},
			big, nil,
		},
		{
			"two aliases flipping give one flip",
			PlanNode{NodeType: "Nested Loop", Plans: []PlanNode{
				scanNode("Index Scan", "public", "orders", "a"),
				scanNode("Index Scan", "public", "orders", "b"),
			}},
			PlanNode{NodeType: "Nested Loop", Plans: []PlanNode{
				scanNode("Seq Scan", "public", "orders", "a"),
				scanNode("Seq Scan", "public", "orders", "b"),
			}},
			big, []genericFlip{orders},
		},
		{
			"self join skipped",
			PlanNode{NodeType: "Nested Loop", Plans: []PlanNode{
				scanNode("Index Scan", "public", "orders", "orders"),
				scanNode("Seq Scan", "public", "orders", "orders"),
			}},
			scanNode("Seq Scan", "public", "orders", "orders"),
			big, nil,
		},
		{
			"different aliases kept apart",
			PlanNode{NodeType: "Nested Loop", Plans: []PlanNode{
				scanNode("Index Scan", "public", "orders", "a"),
				scanNode("Seq Scan", "public", "orders", "b"),
			}},
			PlanNode{NodeType: "Nested Loop", Plans: []PlanNode{
				scanNode("Seq Scan", "public", "orders", "a"),
				scanNode("Seq Scan", "public", "orders", "b"),
			}},
			big, []genericFlip{orders},
		},
		{
			"sorted by relation",
			PlanNode{NodeType: "Nested Loop", Plans: []PlanNode{
				scanNode("Index Scan", "public", "users", "users"),
				scanNode("Index Scan", "public", "orders", "orders"),
			}},
			PlanNode{NodeType: "Hash Join", Plans: []PlanNode{
				scanNode("Seq Scan", "public", "users", "users"),
				scanNode("Seq Scan", "public", "orders", "orders"),
			}},
			big, []genericFlip{orders, {Schema: "public", Relation: "users"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := genericScanFlips(tt.custom, tt.generic, tt.relpages)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("flips = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenericVerdict(t *testing.T) {
	orders := []genericFlip{{Schema: "public", Relation: "orders"}}
	bindings := []string{"1_broad", "2_broad", "6_selective"}

	tests := []struct {
		name       string
		settled    bool
		flips      map[string][]genericFlip
		wantFailed bool
		want       string
	}{
		{"settled, flip", true, map[string][]genericFlip{"1_broad": orders}, true, `generic: orders index scan -> Seq Scan; binding "1_broad"`},
		{"settled, flips on two", true, map[string][]genericFlip{"2_broad": orders, "6_selective": orders}, true, `generic: orders index scan -> Seq Scan; binding "2_broad" (+1 other bindings)`},
		{"settled, no flip", true, nil, false, "settled; no index->seq flip"},
		{"not settled, flip", false, map[string][]genericFlip{"1_broad": orders}, false, "generic plan would Seq Scan orders; settles if broad bindings dominate"},
		{"not settled, no flip", false, nil, false, "stays custom at execution 6"},
		{"settled, empty slice", true, map[string][]genericFlip{"1_broad": {}}, false, "settled; no index->seq flip"},
		{"settled, three bindings flip", true, map[string][]genericFlip{"1_broad": orders, "2_broad": orders, "6_selective": orders}, true, `generic: orders index scan -> Seq Scan; binding "1_broad" (+2 other bindings)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failed, msg := genericVerdict(tt.settled, bindings, tt.flips)
			if failed != tt.wantFailed || msg != tt.want {
				t.Errorf("got (%v, %q), want (%v, %q)", failed, msg, tt.wantFailed, tt.want)
			}
		})
	}
}

func TestGenericVerdictNoBindings(t *testing.T) {
	failed, msg := genericVerdict(true, nil, nil)
	if failed || msg != "settled; no index->seq flip" {
		t.Errorf("got (%v, %q)", failed, msg)
	}
}
