package regresql

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type ConsoleOptions struct {
	Color    bool
	NoColor  bool
	FullDiff bool
	NoDiff   bool
}

type ConsoleFormatter struct {
	results  []TestResult
	options  ConsoleOptions
	useColor bool
}

func (f *ConsoleFormatter) SetOptions(opts ConsoleOptions) {
	f.options = opts
	f.useColor = f.shouldUseColor()
}

func (f *ConsoleFormatter) shouldUseColor() bool {
	// Explicit flags take precedence
	if f.options.NoColor {
		return false
	}
	if f.options.Color {
		return true
	}

	// Respect NO_COLOR environment variable (https://no-color.org/)
	if _, exists := os.LookupEnv("NO_COLOR"); exists {
		return false
	}

	// Check TERM for dumb terminals
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	// Auto-detect: use color if stdout is a terminal
	return term.IsTerminal(int(os.Stdout.Fd()))
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
)

func (f *ConsoleFormatter) colorize(text, color string) string {
	if !f.useColor {
		return text
	}
	return color + text + colorReset
}

func (f *ConsoleFormatter) Start(w io.Writer) error {
	fmt.Fprintln(w, "\nRunning regression tests...")
	f.results = make([]TestResult, 0)
	return nil
}

func (f *ConsoleFormatter) AddResult(r TestResult, w io.Writer) error {
	f.results = append(f.results, r)

	// Show progress indicator with color
	switch r.Status {
	case "passed":
		fmt.Fprint(w, f.colorize(".", colorGreen))
	case "failed":
		fmt.Fprint(w, f.colorize("F", colorRed))
	case "pending":
		fmt.Fprint(w, f.colorize("?", colorCyan))
	case "warning":
		fmt.Fprint(w, f.colorize("W", colorYellow))
	case "skipped":
		fmt.Fprint(w, f.colorize("S", colorDim))
	}
	return nil
}

func (f *ConsoleFormatter) printCostFailure(r TestResult, w io.Writer) {
	if r.AnalyzeMode {
		fmt.Fprintf(w, "  Expected buffers: %d\n", r.BaselineBuffers)
		fmt.Fprintf(w, "  Actual buffers:   %d (+%.1f%%)\n", r.ActualBuffers, r.BufferIncrease)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  Cost (info):      %.2f (baseline: %.2f)\n", r.ActualCost, r.ExpectedCost)
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "  Expected cost: %.2f\n", r.ExpectedCost)
		fmt.Fprintf(w, "  Actual cost:   %.2f (+%.1f%%)\n", r.ActualCost, r.PercentIncrease)
		fmt.Fprintln(w)
	}

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
	maxLines := 5
	if f.options.FullDiff {
		maxLines = len(lines) // No limit
	}

	for _, line := range lines {
		if shown >= maxLines {
			break
		}
		// Show hunk headers to separate different change locations
		if strings.HasPrefix(line, "@@") {
			fmt.Fprintf(w, "  %s\n", f.colorize(line, colorCyan))
			// Don't count headers toward the limit
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			fmt.Fprintf(w, "  %s\n", f.colorize(line, colorGreen))
			shown++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			fmt.Fprintf(w, "  %s\n", f.colorize(line, colorRed))
			shown++
		}
	}
	if !f.options.FullDiff && shown >= maxLines {
		fmt.Fprintln(w, f.colorize("  ... (use --diff to see full output)", colorDim))
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
		fmt.Fprintln(w, f.colorize("  ⚠️  Rows are identical but in different order.", colorYellow))
		fmt.Fprintln(w, "  Consider adding ORDER BY clause to ensure deterministic results.")

	case DiffTypeRowCount, DiffTypeMultiple:
		if diff.RemovedRows > 0 && diff.AddedRows > 0 {
			fmt.Fprintf(w, "  ├─ Matching: %d rows\n", diff.MatchingRows)
			fmt.Fprintf(w, "  ├─ %s\n", f.colorize(fmt.Sprintf("Added:    %d rows", diff.AddedRows), colorGreen))
			fmt.Fprintf(w, "  └─ %s\n", f.colorize(fmt.Sprintf("Removed:  %d rows", diff.RemovedRows), colorRed))
		} else if diff.RemovedRows > 0 {
			fmt.Fprintf(w, "  └─ Result:   %s\n", f.colorize(fmt.Sprintf("%d rows removed", diff.RemovedRows), colorRed))
		} else if diff.AddedRows > 0 {
			fmt.Fprintf(w, "  └─ Result:   %s\n", f.colorize(fmt.Sprintf("%d rows added", diff.AddedRows), colorGreen))
		}

		fmt.Fprintln(w)

		if len(diff.RemovedSamples) > 0 {
			fmt.Fprintf(w, "  %s\n", f.colorize(fmt.Sprintf("REMOVED ROWS (showing %d of %d):", len(diff.RemovedSamples), diff.RemovedRows), colorRed))
			for _, row := range diff.RemovedSamples {
				fmt.Fprintf(w, "  %s\n", f.colorize(f.formatRow(diff.Columns, row), colorRed))
			}
			fmt.Fprintln(w)
		}

		if len(diff.AddedSamples) > 0 {
			fmt.Fprintf(w, "  %s\n", f.colorize(fmt.Sprintf("ADDED ROWS (showing %d of %d):", len(diff.AddedSamples), diff.AddedRows), colorGreen))
			for _, row := range diff.AddedSamples {
				fmt.Fprintf(w, "  %s\n", f.colorize(f.formatRow(diff.Columns, row), colorGreen))
			}
			fmt.Fprintln(w)
		}

	case DiffTypeValues:
		fmt.Fprintf(w, "  ├─ Matching: %d rows\n", diff.MatchingRows)
		fmt.Fprintf(w, "  └─ %s\n", f.colorize(fmt.Sprintf("Modified: %d rows", diff.ModifiedRows), colorYellow))
		fmt.Fprintln(w)

		if len(diff.ModifiedSamples) > 0 {
			fmt.Fprintf(w, "  %s\n", f.colorize(fmt.Sprintf("MODIFIED ROWS (showing %d of %d):", len(diff.ModifiedSamples), diff.ModifiedRows), colorYellow))
			for i, sample := range diff.ModifiedSamples {
				fmt.Fprintf(w, "  Row #%d:\n", i+1)
				fmt.Fprintf(w, "    %s %s\n", f.colorize("Expected:", colorRed), f.formatRow(diff.Columns, sample.ExpectedRow))
				fmt.Fprintf(w, "    %s %s\n", f.colorize("Actual:  ", colorGreen), f.formatRow(diff.Columns, sample.ActualRow))
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
			fmt.Fprintf(w, "  %s  %s\n", f.colorize("⚠️", colorYellow), warning.Message)
			if warning.Suggestion != "" {
				fmt.Fprintf(w, "    %s %s\n", f.colorize("Suggestion:", colorDim), warning.Suggestion)
			}
		}
	}
}

func (f *ConsoleFormatter) Finish(s *TestSummary, w io.Writer) error {
	fmt.Fprintln(w) // End progress line
	fmt.Fprintln(w)

	// Summary section
	fmt.Fprintln(w, "RESULTS:")
	fmt.Fprintf(w, "  %s %d passing\n", f.colorize("✓", colorGreen), s.Passed)
	if s.Failed > 0 {
		fmt.Fprintf(w, "  %s %d failing\n", f.colorize("✗", colorRed), s.Failed)
	}
	if s.Pending > 0 {
		fmt.Fprintf(w, "  %s %d pending (no baseline)\n", f.colorize("?", colorCyan), s.Pending)
	}
	if s.Skipped > 0 {
		fmt.Fprintf(w, "  %s %d skipped\n", f.colorize("-", colorDim), s.Skipped)
	}
	fmt.Fprintf(w, "  %.2fs total\n", s.Duration)

	// Failing tests details
	if s.Failed > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, f.colorize("FAILING:", colorRed))
		for _, r := range f.results {
			if r.Status == "failed" {
				fmt.Fprintf(w, "  %s\n", r.Name)
				if !f.options.NoDiff {
					if r.Type == "cost" {
						f.printCostFailure(r, w)
					} else if r.Type == "output" {
						f.printOutputDiff(r, w)
					}
				}
				if r.Error != "" {
					fmt.Fprintf(w, "    %s %s\n", f.colorize("Error:", colorRed), r.Error)
				}
			}
		}
	}

	// Warnings (passed tests with plan warnings)
	var warnings []TestResult
	for _, r := range f.results {
		if r.Status == "passed" && len(r.PlanWarnings) > 0 {
			warnings = append(warnings, r)
		}
	}
	if len(warnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, f.colorize("WARNINGS:", colorYellow))
		for _, r := range warnings {
			fmt.Fprintf(w, "  %s\n", r.Name)
			f.printWarnings(r.PlanWarnings, w)
		}
	}

	// Pending tests
	if s.Pending > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, f.colorize("PENDING (no baseline):", colorCyan))
		for _, r := range f.results {
			if r.Status == "pending" {
				fmt.Fprintf(w, "  %s\n", r.Name)
			}
		}
	}

	// Suggestions
	f.printSuggestions(s, w)

	return nil
}

func (f *ConsoleFormatter) printSuggestions(s *TestSummary, w io.Writer) {
	if s.Failed == 0 && s.Pending == 0 {
		return
	}

	fmt.Fprintln(w)

	if s.Failed > 0 {
		fmt.Fprintln(w, "To accept changes: regresql update <query-name>")
	}
	if s.Pending > 0 {
		fmt.Fprintln(w, "To create baselines: regresql update --pending")
	}
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
