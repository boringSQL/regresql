package regresql

import (
	"fmt"
	"io"
	"strings"
)

type ConsoleFormatter struct{
	lastQueryGroup string
}

func (f *ConsoleFormatter) Start(w io.Writer) error {
	fmt.Fprintln(w, "\nRunning regression tests...")
	f.lastQueryGroup = ""
	return nil
}

func (f *ConsoleFormatter) AddResult(r TestResult, w io.Writer) error {
	queryGroup := f.extractQueryGroup(r.Name)
	if queryGroup != f.lastQueryGroup && f.lastQueryGroup != "" {
		fmt.Fprintln(w)
	}
	f.lastQueryGroup = queryGroup

	switch r.Status {
	case "passed":
		fmt.Fprintf(w, "✓ %s (%.2fs)\n", r.Name, r.Duration)
		f.printWarnings(r.PlanWarnings, w)

	case "failed":
		fmt.Fprintf(w, "✗ %s (%.2fs)\n", r.Name, r.Duration)
		if r.Type == "cost" {
			f.printCostFailure(r, w)
		} else if r.Type == "output" {
			f.printOutputDiff(r, w)
		}
		if r.Error != "" {
			fmt.Fprintf(w, "  Error: %s\n", r.Error)
		}
		fmt.Fprintln(w)

	case "warning":
		fmt.Fprintf(w, "⚠️  %s (%.2fs)\n", r.Name, r.Duration)
		f.printWarnings(r.PlanWarnings, w)
		fmt.Fprintln(w)

	case "skipped":
		return nil
	}
	return nil
}

func (f *ConsoleFormatter) extractQueryGroup(testName string) string {
	parts := strings.Split(testName, ".")
	if len(parts) <= 1 {
		return testName
	}
	return parts[0]
}

func (f *ConsoleFormatter) printCostFailure(r TestResult, w io.Writer) {
	fmt.Fprintf(w, "  Expected cost: %.2f\n", r.ExpectedCost)
	fmt.Fprintf(w, "  Actual cost:   %.2f (+%.1f%%)\n", r.ActualCost, r.PercentIncrease)
	fmt.Fprintln(w)

	if len(r.PlanRegressions) > 0 {
		f.printPlanRegressions(r.PlanRegressions, w)
	} else if !r.PlanChanged {
		fmt.Fprintln(w, "  Likely cause: Data distribution changed or outdated statistics")
		fmt.Fprintln(w, "  Try: ANALYZE table_name;")
	}
}

func (f *ConsoleFormatter) printPlanRegressions(regressions []PlanRegression, w io.Writer) {
	hasCritical := hasAnyCritical(regressions)
	if hasCritical {
		fmt.Fprintln(w, "  ⚠️  PLAN REGRESSION DETECTED:")
	}

	for _, reg := range regressions {
		symbol := GetSeveritySymbol(reg.Severity)
		if reg.Table != "" {
			fmt.Fprintf(w, "  %s Table '%s': %s → %s\n", symbol, reg.Table, reg.OldScan, reg.NewScan)
		} else {
			fmt.Fprintf(w, "  %s %s\n", symbol, reg.Message)
		}

		if reg.Severity == "critical" && len(reg.Recommendations) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "  Recommendations:")
			for _, rec := range reg.Recommendations {
				fmt.Fprintf(w, "  %s\n", rec)
			}
		}
	}
	fmt.Fprintln(w)
}

func (f *ConsoleFormatter) printOutputDiff(r TestResult, w io.Writer) {
	// Use structured diff if available
	if r.StructuredDiff != nil {
		f.printStructuredDiff(r.StructuredDiff, w)
		return
	}

	// Fall back to text diff
	if r.Diff == "" {
		return
	}

	lines := strings.Split(r.Diff, "\n")
	shown := 0
	for _, line := range lines {
		if shown >= 5 {
			break
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") ||
			strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			fmt.Fprintf(w, "  %s\n", line)
			shown++
		}
	}
	if shown >= 5 {
		fmt.Fprintln(w, "  ...")
	}
}

func (f *ConsoleFormatter) printStructuredDiff(diff *StructuredDiff, w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  COMPARISON SUMMARY:")
	fmt.Fprintf(w, "  ├─ Expected: %d rows\n", diff.ExpectedRows)
	fmt.Fprintf(w, "  ├─ Actual:   %d rows\n", diff.ActualRows)

	switch diff.Type {
	case DiffTypeOrdering:
		fmt.Fprintln(w, "  └─ Result:   Same data, different order")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  ⚠️  Rows are identical but in different order.")
		fmt.Fprintln(w, "  Consider adding ORDER BY clause to ensure deterministic results.")

	case DiffTypeRowCount, DiffTypeMultiple:
		if diff.RemovedRows > 0 && diff.AddedRows > 0 {
			fmt.Fprintf(w, "  ├─ Matching: %d rows\n", diff.MatchingRows)
			fmt.Fprintf(w, "  ├─ Added:    %d rows\n", diff.AddedRows)
			fmt.Fprintf(w, "  └─ Removed:  %d rows\n", diff.RemovedRows)
		} else if diff.RemovedRows > 0 {
			fmt.Fprintf(w, "  └─ Result:   %d rows removed\n", diff.RemovedRows)
		} else if diff.AddedRows > 0 {
			fmt.Fprintf(w, "  └─ Result:   %d rows added\n", diff.AddedRows)
		}

		fmt.Fprintln(w)

		if len(diff.RemovedSamples) > 0 {
			fmt.Fprintf(w, "  REMOVED ROWS (showing %d of %d):\n", len(diff.RemovedSamples), diff.RemovedRows)
			for _, row := range diff.RemovedSamples {
				fmt.Fprintf(w, "  %s\n", f.formatRow(diff.Columns, row))
			}
			fmt.Fprintln(w)
		}

		if len(diff.AddedSamples) > 0 {
			fmt.Fprintf(w, "  ADDED ROWS (showing %d of %d):\n", len(diff.AddedSamples), diff.AddedRows)
			for _, row := range diff.AddedSamples {
				fmt.Fprintf(w, "  %s\n", f.formatRow(diff.Columns, row))
			}
			fmt.Fprintln(w)
		}

	case DiffTypeValues:
		fmt.Fprintf(w, "  ├─ Matching: %d rows\n", diff.MatchingRows)
		fmt.Fprintf(w, "  └─ Modified: %d rows\n", diff.ModifiedRows)
		fmt.Fprintln(w)

		if len(diff.ModifiedSamples) > 0 {
			fmt.Fprintf(w, "  MODIFIED ROWS (showing %d of %d):\n", len(diff.ModifiedSamples), diff.ModifiedRows)
			for i, sample := range diff.ModifiedSamples {
				fmt.Fprintf(w, "  Row #%d:\n", i+1)
				fmt.Fprintf(w, "    Expected: %s\n", f.formatRow(diff.Columns, sample.ExpectedRow))
				fmt.Fprintf(w, "    Actual:   %s\n", f.formatRow(diff.Columns, sample.ActualRow))
			}
		}
	}
}

func (f *ConsoleFormatter) formatRow(columns []string, row []any) string {
	pairs := make([]string, len(columns))
	for i, col := range columns {
		if i < len(row) {
			pairs[i] = fmt.Sprintf("%s: %v", col, formatValue(row[i]))
		}
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}

func formatValue(v any) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return fmt.Sprintf(`"%s"`, val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%.2f", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func (f *ConsoleFormatter) printWarnings(warnings []PlanWarning, w io.Writer) {
	for _, warning := range warnings {
		if warning.Severity == "warning" {
			fmt.Fprintf(w, "  ⚠️  %s\n", warning.Message)
			if warning.Suggestion != "" {
				fmt.Fprintf(w, "    Suggestion: %s\n", warning.Suggestion)
			}
		}
	}
}

func (f *ConsoleFormatter) Finish(s *TestSummary, w io.Writer) error {
	fmt.Fprintln(w)
	if s.Failed > 0 || s.Skipped > 0 {
		fmt.Fprintf(w, "Results: %d passed, %d failed", s.Passed, s.Failed)
		if s.Skipped > 0 {
			fmt.Fprintf(w, ", %d skipped", s.Skipped)
		}
		fmt.Fprintf(w, " (%.2fs)\n", s.Duration)
	} else {
		fmt.Fprintf(w, "Results: %d passed (%.2fs)\n", s.Passed, s.Duration)
	}
	return nil
}

func hasAnyCritical(regressions []PlanRegression) bool {
	for _, reg := range regressions {
		if reg.Severity == "critical" {
			return true
		}
	}
	return false
}

func init() {
	RegisterFormatter("console", &ConsoleFormatter{})
}
