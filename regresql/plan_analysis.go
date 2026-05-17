package regresql

import (
	"fmt"
	"regexp"
	"strings"
)

type (
	PlanSignature struct {
		NodeTypes   []string
		Relations   map[string]ScanInfo
		IndexesUsed []string
		HasSeqScan  bool
		HasSort     bool
		JoinTypes   []string
	}

	ScanInfo struct {
		ScanType  string
		IndexName string
		IndexCond string
		Filter    string
	}
)

var (
	joinNodeTypes = map[string]bool{
		"Nested Loop": true,
		"Hash Join":   true,
		"Merge Join":  true,
	}

	indexScanTypes = map[string]bool{
		"Index Scan":        true,
		"Index Only Scan":   true,
		"Bitmap Index Scan": true,
	}

	simpleColumnPattern = regexp.MustCompile(`\(([a-zA-Z_][a-zA-Z0-9_]*)\s*=`)
)

// ExtractPlanSignatureFromNode extracts plan signature from typed PlanNode
func ExtractPlanSignatureFromNode(node *PlanNode) *PlanSignature {
	sig := &PlanSignature{
		Relations: make(map[string]ScanInfo),
	}
	extractFromTypedNode(node, sig)
	return sig
}

func extractFromTypedNode(node *PlanNode, sig *PlanSignature) {
	if node.NodeType != "" {
		sig.NodeTypes = append(sig.NodeTypes, node.NodeType)

		if node.NodeType == "Seq Scan" {
			sig.HasSeqScan = true
		}
		if node.NodeType == "Sort" {
			sig.HasSort = true
		}
		if joinNodeTypes[node.NodeType] {
			sig.JoinTypes = append(sig.JoinTypes, node.NodeType)
		}
	}

	if node.RelationName != "" {
		scanInfo := ScanInfo{
			ScanType:  node.NodeType,
			IndexName: node.IndexName,
			IndexCond: node.IndexCond,
			Filter:    node.Filter,
		}

		// Bitmap Heap Scan's index name lives on child Bitmap Index Scan nodes.
		if node.NodeType == "Bitmap Heap Scan" {
			bitmapIndexes := collectBitmapIndexNamesTyped(node)
			if len(bitmapIndexes) > 0 {
				scanInfo.IndexName = strings.Join(bitmapIndexes, ", ")
				sig.IndexesUsed = append(sig.IndexesUsed, bitmapIndexes...)
			}
		} else if scanInfo.IndexName != "" {
			sig.IndexesUsed = append(sig.IndexesUsed, scanInfo.IndexName)
		}

		sig.Relations[node.RelationName] = scanInfo
	}

	for i := range node.Plans {
		extractFromTypedNode(&node.Plans[i], sig)
	}
}

func collectBitmapIndexNamesTyped(node *PlanNode) []string {
	var names []string
	for i := range node.Plans {
		child := &node.Plans[i]
		if child.NodeType == "Bitmap Index Scan" {
			if child.IndexName != "" {
				names = append(names, child.IndexName)
			}
		} else if child.NodeType == "BitmapAnd" || child.NodeType == "BitmapOr" {
			names = append(names, collectBitmapIndexNamesTyped(child)...)
		}
	}
	return names
}

// ExtractPlanSignature extracts plan signature from untyped map (for backwards compatibility)
func ExtractPlanSignature(explainPlan map[string]any) (*PlanSignature, error) {
	planData, ok := explainPlan["Plan"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid EXPLAIN plan structure: missing Plan key")
	}

	sig := &PlanSignature{
		Relations: make(map[string]ScanInfo),
	}

	extractFromNode(planData, sig)
	return sig, nil
}

func extractFromNode(node map[string]any, sig *PlanSignature) {
	nodeType := getString(node, "Node Type")
	if nodeType != "" {
		sig.NodeTypes = append(sig.NodeTypes, nodeType)

		if nodeType == "Seq Scan" {
			sig.HasSeqScan = true
		}
		if nodeType == "Sort" {
			sig.HasSort = true
		}
		if joinNodeTypes[nodeType] {
			sig.JoinTypes = append(sig.JoinTypes, nodeType)
		}
	}

	if relationName := getString(node, "Relation Name"); relationName != "" {
		scanInfo := ScanInfo{
			ScanType:  nodeType,
			IndexName: getString(node, "Index Name"),
			IndexCond: getString(node, "Index Cond"),
			Filter:    getString(node, "Filter"),
		}

		if nodeType == "Bitmap Heap Scan" {
			bitmapIndexes := collectBitmapIndexNames(node)
			if len(bitmapIndexes) > 0 {
				scanInfo.IndexName = strings.Join(bitmapIndexes, ", ")
				sig.IndexesUsed = append(sig.IndexesUsed, bitmapIndexes...)
			}
		} else if scanInfo.IndexName != "" {
			sig.IndexesUsed = append(sig.IndexesUsed, scanInfo.IndexName)
		}

		sig.Relations[relationName] = scanInfo
	}

	if plans, ok := node["Plans"].([]any); ok {
		for _, p := range plans {
			if childNode, ok := p.(map[string]any); ok {
				extractFromNode(childNode, sig)
			}
		}
	}
}

func collectBitmapIndexNames(node map[string]any) []string {
	var names []string
	plans, ok := node["Plans"].([]any)
	if !ok {
		return names
	}
	for _, p := range plans {
		child, ok := p.(map[string]any)
		if !ok {
			continue
		}
		switch getString(child, "Node Type") {
		case "Bitmap Index Scan":
			if name := getString(child, "Index Name"); name != "" {
				names = append(names, name)
			}
		case "BitmapAnd", "BitmapOr":
			names = append(names, collectBitmapIndexNames(child)...)
		}
	}
	return names
}

func getString(m map[string]any, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func ExtractSimpleColumn(indexCond string) string {
	if matches := simpleColumnPattern.FindStringSubmatch(indexCond); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func FormatScanDescription(scanInfo ScanInfo) string {
	if scanInfo.IndexName != "" {
		return fmt.Sprintf("%s using %s", scanInfo.ScanType, scanInfo.IndexName)
	}
	return scanInfo.ScanType
}

func CompareScans(baseline, current ScanInfo) bool {
	return baseline.ScanType == current.ScanType && baseline.IndexName == current.IndexName
}

func IsIndexScan(scanType string) bool {
	return indexScanTypes[scanType]
}

func HasPlanChanged(baseline, current *PlanSignature) bool {
	if len(baseline.Relations) != len(current.Relations) {
		return true
	}

	for tableName, baselineScan := range baseline.Relations {
		currentScan, exists := current.Relations[tableName]
		if !exists || !CompareScans(baselineScan, currentScan) {
			return true
		}
	}

	if len(baseline.JoinTypes) != len(current.JoinTypes) {
		return true
	}

	for i := range baseline.JoinTypes {
		if baseline.JoinTypes[i] != current.JoinTypes[i] {
			return true
		}
	}

	return false
}

func FormatIndexesUsed(indexes []string) string {
	if len(indexes) == 0 {
		return "none"
	}
	return strings.Join(indexes, ", ")
}
