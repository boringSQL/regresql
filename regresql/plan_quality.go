package regresql

import (
	"fmt"
	"strings"
)

type (
	PlanWarning struct {
		Type       WarningType
		Severity   string
		Message    string
		Table      string
		Suggestion string
		Details    string
	}

	WarningType string
)

const (
	SeqScanDetected        WarningType = "sequential_scan_detected"
	MultipleSeqScans       WarningType = "multiple_sequential_scans"
	MultipleSorts          WarningType = "multiple_sorts"
	NestedLoopWithSeqScan  WarningType = "nested_loop_with_seqscan"
	SeqScanOnCriticalTable WarningType = "seq_scan_critical_table"
)

// Queries below these thresholds skip scan-related warnings.
// Seq scan on tiny tables is the correct plan choice.
const (
	lowCostThreshold         = 10.0
	lowBufferThreshold int64 = 10 // shared buffers (8KB pages)
)

type PlanCostInfo struct {
	TotalCost    float64
	TotalBuffers int64 // shared_hit + shared_read; -1 if unavailable
}

func DetectPlanQualityIssues(sig *PlanSignature, opts RegressQLOptions, ignoredTables, criticalTables []string, cost PlanCostInfo) []PlanWarning {
	var warnings []PlanWarning

	// Skip scan/join warnings for trivially cheap queries — seq scan on
	// small tables is optimal and warning about it is noise.
	lowCost := cost.TotalCost > 0 && cost.TotalCost < lowCostThreshold
	lowBuffers := cost.TotalBuffers >= 0 && cost.TotalBuffers < lowBufferThreshold
	trivial := lowCost || lowBuffers

	// Critical-table seq scans bypass both the trivial-cost filter and the
	// ignore list
	if sig.HasSeqScan && !opts.NoSeqScanWarn && len(criticalTables) > 0 {
		seqTables := findSeqScanTables(sig.Relations)
		critical := intersectTables(seqTables, criticalTables)
		for _, t := range critical {
			warnings = append(warnings, PlanWarning{
				Type:       SeqScanOnCriticalTable,
				Severity:   "error",
				Table:      t,
				Message:    fmt.Sprintf("Sequential scan on critical table '%s'", t),
				Suggestion: "Add an index covering the filter/join columns, or remove this table from plan_quality.critical_tables",
				Details:    fmt.Sprintf("Table '%s' is listed in plan_quality.critical_tables; seq scans on it are treated as errors", t),
			})
		}
	}

	if sig.HasSeqScan && !opts.NoSeqScanWarn && !trivial {
		seqScanTables := filterIgnoredTables(
			filterCriticalTables(findSeqScanTables(sig.Relations), criticalTables),
			ignoredTables,
		)

		switch len(seqScanTables) {
		case 1:
			warnings = append(warnings, PlanWarning{
				Type:       SeqScanDetected,
				Severity:   "warning",
				Table:      seqScanTables[0],
				Message:    fmt.Sprintf("Sequential scan detected on table '%s'", seqScanTables[0]),
				Suggestion: "Consider adding an index if this table is large or this query is frequently executed",
				Details:    fmt.Sprintf("Table '%s' is being scanned sequentially, which may be slow on large tables", seqScanTables[0]),
			})
		case 0:
			// All seq scans are on ignored tables
		default:
			warnings = append(warnings, PlanWarning{
				Type:       MultipleSeqScans,
				Severity:   "warning",
				Message:    fmt.Sprintf("Multiple sequential scans detected on tables: %s", strings.Join(seqScanTables, ", ")),
				Suggestion: "Review query and consider adding indexes on filtered/joined columns",
				Details:    fmt.Sprintf("%d tables are being scanned sequentially", len(seqScanTables)),
			})
		}
	}

	if sortCount := countNodes(sig.NodeTypes, "Sort"); sortCount > 1 {
		warnings = append(warnings, PlanWarning{
			Type:       MultipleSorts,
			Severity:   "warning",
			Message:    fmt.Sprintf("Multiple sort operations detected (%d sorts)", sortCount),
			Suggestion: "Consider composite indexes for ORDER BY clauses to avoid sorting",
			Details:    "Multiple sorts can be expensive; indexes can eliminate or reduce sorting",
		})
	}

	if hasNestedLoopWithSeqScan(sig) && !trivial {
		warnings = append(warnings, PlanWarning{
			Type:       NestedLoopWithSeqScan,
			Severity:   "warning",
			Message:    "Nested loop join with sequential scan detected",
			Suggestion: "Add index on join column to avoid repeated sequential scans",
			Details:    "Nested loops with seq scans can be very slow; the inner table is scanned repeatedly",
		})
	}

	return warnings
}

func findSeqScanTables(relations map[string]ScanInfo) []string {
	var tables []string
	for tableName, scanInfo := range relations {
		if scanInfo.ScanType == "Seq Scan" {
			tables = append(tables, tableName)
		}
	}
	return tables
}

func countNodes(nodeTypes []string, targetType string) int {
	count := 0
	for _, nt := range nodeTypes {
		if nt == targetType {
			count++
		}
	}
	return count
}

func hasNestedLoopWithSeqScan(sig *PlanSignature) bool {
	if !sig.HasSeqScan {
		return false
	}
	for _, jt := range sig.JoinTypes {
		if jt == "Nested Loop" {
			return true
		}
	}
	return false
}

func intersectTables(tables, wanted []string) []string {
	if len(wanted) == 0 || len(tables) == 0 {
		return nil
	}
	wantedMap := make(map[string]bool, len(wanted))
	for _, t := range wanted {
		wantedMap[t] = true
	}
	out := make([]string, 0, len(tables))
	for _, t := range tables {
		if wantedMap[t] {
			out = append(out, t)
		}
	}
	return out
}

// filterCriticalTables removes tables that are already being reported as
// critical, so the generic seq-scan rule does not double-report them.
func filterCriticalTables(tables, criticalTables []string) []string {
	if len(criticalTables) == 0 {
		return tables
	}
	critMap := make(map[string]bool, len(criticalTables))
	for _, t := range criticalTables {
		critMap[t] = true
	}
	out := make([]string, 0, len(tables))
	for _, t := range tables {
		if !critMap[t] {
			out = append(out, t)
		}
	}
	return out
}

func filterIgnoredTables(tables, ignoredTables []string) []string {
	if len(ignoredTables) == 0 {
		return tables
	}

	ignoreMap := make(map[string]bool, len(ignoredTables))
	for _, t := range ignoredTables {
		ignoreMap[t] = true
	}

	filtered := make([]string, 0, len(tables))
	for _, table := range tables {
		if !ignoreMap[table] {
			filtered = append(filtered, table)
		}
	}
	return filtered
}
