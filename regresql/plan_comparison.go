package regresql

import (
	"fmt"
	"strings"
)

type (
	PlanRegression struct {
		Type            RegressionType
		Table           string
		OldScan         string
		NewScan         string
		IndexName       string
		IndexCond       string
		Severity        string // "critical", "warning", "info"
		Message         string
		Recommendations []string
	}

	RegressionType string
)

const (
	IndexToSeqScan     RegressionType = "index_to_seqscan"
	IndexOnlyToIndex   RegressionType = "index_only_to_index"
	JoinTypeChanged    RegressionType = "join_type_changed"
	SortAdded          RegressionType = "sort_added"
	IndexChanged       RegressionType = "index_changed"
	TableAccessChanged RegressionType = "table_access_changed"
)

func DetectPlanRegressions(baseline, current *PlanSignature) []PlanRegression {
	var regressions []PlanRegression

	for tableName, baselineScan := range baseline.Relations {
		currentScan, exists := current.Relations[tableName]
		if !exists {
			continue
		}

		if regression := compareScanMethods(tableName, baselineScan, currentScan); regression != nil {
			regressions = append(regressions, *regression)
		}
	}

	if len(baseline.JoinTypes) != len(current.JoinTypes) {
		for i := range baseline.JoinTypes {
			if i >= len(current.JoinTypes) || baseline.JoinTypes[i] != current.JoinTypes[i] {
				if regression := compareJoinTypes(baseline.JoinTypes, current.JoinTypes); regression != nil {
					regressions = append(regressions, *regression)
				}
				break
			}
		}
	}

	if !baseline.HasSort && current.HasSort {
		regressions = append(regressions, PlanRegression{
			Type:     SortAdded,
			Severity: "warning",
			Message:  "Sort operation added to query plan",
			Recommendations: []string{
				"-- Sort operation may indicate missing index for ORDER BY",
				"-- Review query ORDER BY clause and consider adding appropriate index",
			},
		})
	}

	return regressions
}

func compareScanMethods(tableName string, baseline, current ScanInfo) *PlanRegression {
	if IsIndexScan(baseline.ScanType) && current.ScanType == "Seq Scan" {
		return &PlanRegression{
			Type:            IndexToSeqScan,
			Table:           tableName,
			OldScan:         FormatScanDescription(baseline),
			NewScan:         FormatScanDescription(current),
			IndexName:       baseline.IndexName,
			IndexCond:       baseline.IndexCond,
			Severity:        "critical",
			Message:         fmt.Sprintf("Table '%s' changed from %s to Seq Scan", tableName, baseline.ScanType),
			Recommendations: buildIndexRegressionRecommendations(tableName, baseline),
		}
	}

	if baseline.ScanType == "Index Only Scan" && current.ScanType == "Index Scan" {
		return &PlanRegression{
			Type:      IndexOnlyToIndex,
			Table:     tableName,
			OldScan:   FormatScanDescription(baseline),
			NewScan:   FormatScanDescription(current),
			IndexName: baseline.IndexName,
			Severity:  "warning",
			Message:   fmt.Sprintf("Table '%s' changed from Index Only Scan to Index Scan", tableName),
			Recommendations: []string{
				"-- Index Only Scan degraded to Index Scan",
				"-- This may indicate index bloat or visibility map issues",
				fmt.Sprintf("VACUUM ANALYZE %s;", tableName),
				"-- Consider REINDEX if bloat is significant:",
				fmt.Sprintf("-- REINDEX INDEX %s;", baseline.IndexName),
			},
		}
	}

	if IsIndexScan(baseline.ScanType) && IsIndexScan(current.ScanType) &&
		baseline.IndexName != "" && current.IndexName != "" &&
		baseline.IndexName != current.IndexName {
		return &PlanRegression{
			Type:      IndexChanged,
			Table:     tableName,
			OldScan:   FormatScanDescription(baseline),
			NewScan:   FormatScanDescription(current),
			IndexName: baseline.IndexName,
			Severity:  "info",
			Message:   fmt.Sprintf("Table '%s' using different index: %s → %s", tableName, baseline.IndexName, current.IndexName),
			Recommendations: []string{
				"-- Optimizer chose a different index",
				"-- This might be better or worse depending on data distribution",
				fmt.Sprintf("-- Old index: %s", baseline.IndexName),
				fmt.Sprintf("-- New index: %s", current.IndexName),
				fmt.Sprintf("ANALYZE %s;", tableName),
			},
		}
	}

	if baseline.ScanType != current.ScanType {
		return &PlanRegression{
			Type:     TableAccessChanged,
			Table:    tableName,
			OldScan:  FormatScanDescription(baseline),
			NewScan:  FormatScanDescription(current),
			Severity: "warning",
			Message:  fmt.Sprintf("Table '%s' access method changed: %s → %s", tableName, baseline.ScanType, current.ScanType),
			Recommendations: []string{
				fmt.Sprintf("-- Table access method changed from %s to %s", baseline.ScanType, current.ScanType),
				fmt.Sprintf("ANALYZE %s;", tableName),
			},
		}
	}

	return nil
}

func compareJoinTypes(baseline, current []string) *PlanRegression {
	if len(baseline) == 0 && len(current) == 0 {
		return nil
	}

	return &PlanRegression{
		Type:     JoinTypeChanged,
		Severity: "info",
		Message:  fmt.Sprintf("Join strategy changed: [%s] → [%s]", strings.Join(baseline, ", "), strings.Join(current, ", ")),
		Recommendations: []string{
			"-- Join strategy changed - this may be better or worse",
			"-- Run ANALYZE on joined tables to ensure statistics are up to date",
		},
	}
}

func buildIndexRegressionRecommendations(tableName string, baseline ScanInfo) []string {
	var recs []string

	recs = append(recs,
		"-- Step 1: Check if index exists",
		fmt.Sprintf("SELECT indexname, indexdef FROM pg_indexes WHERE indexname = '%s';", baseline.IndexName),
		"",
		"-- Step 2: Check table statistics freshness",
		"SELECT schemaname, tablename, last_analyze, last_autoanalyze, n_live_tup",
		fmt.Sprintf("FROM pg_stat_user_tables WHERE tablename = '%s';", tableName),
		"",
		"-- Step 3: Update statistics (always safe)",
		fmt.Sprintf("ANALYZE %s;", tableName),
	)

	if column := ExtractSimpleColumn(baseline.IndexCond); column != "" {
		recs = append(recs,
			"",
			"-- Step 4: If index is missing, recreate it",
			fmt.Sprintf("CREATE INDEX %s ON %s(%s);", baseline.IndexName, tableName, column),
		)
	} else {
		recs = append(recs,
			"",
			"-- Step 4: If index is missing, check the original definition",
			fmt.Sprintf("-- Index condition was: %s", baseline.IndexCond),
			"-- Recreate the index based on the original definition from pg_indexes",
		)
	}

	return recs
}

func GetSeveritySymbol(severity string) string {
	switch severity {
	case "critical":
		return "✗"
	case "warning":
		return "⚠️"
	case "info":
		return "ℹ️"
	default:
		return "•"
	}
}
