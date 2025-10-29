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
		sig.Relations[relationName] = scanInfo

		if scanInfo.IndexName != "" {
			sig.IndexesUsed = append(sig.IndexesUsed, scanInfo.IndexName)
		}
	}

	if plans, ok := node["Plans"].([]any); ok {
		for _, p := range plans {
			if childNode, ok := p.(map[string]any); ok {
				extractFromNode(childNode, sig)
			}
		}
	}
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
