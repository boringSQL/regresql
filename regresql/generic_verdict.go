package regresql

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

const genericMinPages int64 = 1000

type (
	scanKey struct {
		Schema   string
		Relation string
		Alias    string
	}

	genericFlip struct {
		Schema   string
		Relation string
	}
)

func relPagesKey(schema, relation string) string {
	return schema + "." + relation
}

func isIndexedScan(nodeType string) bool {
	switch nodeType {
	case "Index Scan", "Index Only Scan", "Bitmap Heap Scan":
		return true
	}
	return false
}

func collectScans(node PlanNode) map[scanKey]string {
	found := map[scanKey]string{}
	dup := map[scanKey]bool{}

	var walk func(n PlanNode)
	walk = func(n PlanNode) {
		if n.RelationName != "" && strings.HasSuffix(n.NodeType, " Scan") {
			k := scanKey{n.Schema, n.RelationName, n.Alias}
			if _, seen := found[k]; seen {
				dup[k] = true
			}
			found[k] = n.NodeType
		}
		for _, c := range n.Plans {
			walk(c)
		}
	}
	walk(node)

	for k := range dup {
		delete(found, k)
	}
	return found
}

func genericScanFlips(custom, generic PlanNode, relpages map[string]int64) []genericFlip {
	customScans := collectScans(custom)
	genericScans := collectScans(generic)

	var flips []genericFlip
	for k, ct := range customScans {
		if !isIndexedScan(ct) || genericScans[k] != "Seq Scan" {
			continue
		}
		if relpages[relPagesKey(k.Schema, k.Relation)] < genericMinPages {
			continue
		}
		f := genericFlip{Schema: k.Schema, Relation: k.Relation}
		if !slices.Contains(flips, f) {
			flips = append(flips, f)
		}
	}

	sort.Slice(flips, func(i, j int) bool {
		return relPagesKey(flips[i].Schema, flips[i].Relation) < relPagesKey(flips[j].Schema, flips[j].Relation)
	})
	return flips
}

func genericVerdict(settled bool, bindings []string, flips map[string][]genericFlip) (failed bool, message string) {
	first, withFlips := "", 0
	for _, b := range bindings {
		if len(flips[b]) == 0 {
			continue
		}
		if withFlips == 0 {
			first = b
		}
		withFlips++
	}

	switch {
	case withFlips == 0 && settled:
		return false, "settled; no index->seq flip"
	case withFlips == 0:
		return false, "stays custom at execution 6"
	case !settled:
		return false, fmt.Sprintf("generic plan would Seq Scan %s; settles if broad bindings dominate", flips[first][0].Relation)
	}

	msg := fmt.Sprintf("generic: %s index scan -> Seq Scan; binding %q", flips[first][0].Relation, first)
	if withFlips > 1 {
		msg += fmt.Sprintf(" (+%d other bindings)", withFlips-1)
	}
	return true, msg
}
