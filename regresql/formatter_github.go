package regresql

import (
	"fmt"
	"io"
	"strings"
)

type GitHubActionsFormatter struct{}

func (f *GitHubActionsFormatter) Start(w io.Writer) error {
	fmt.Fprintln(w, "::group::Running regression tests")
	return nil
}

func (f *GitHubActionsFormatter) AddResult(r TestResult, w io.Writer) error {
	switch r.Status {
	case "passed":
		// Show plan warnings even for passed tests
		if len(r.PlanWarnings) > 0 {
			for _, warning := range r.PlanWarnings {
				if warning.Severity == "warning" {
					msg := strings.ReplaceAll(warning.Message, "%", "%%")
					fmt.Fprintf(w, "::warning::%s - %s\n", r.Name, msg)
				}
			}
		}
		return nil
	case "failed":
		if r.Type == "cost" {
			// Check for plan regressions
			hasCriticalRegression := false
			var criticalMsg string

			for _, reg := range r.PlanRegressions {
				if reg.Severity == "critical" {
					hasCriticalRegression = true
					if reg.Table != "" {
						criticalMsg = fmt.Sprintf(" - PLAN REGRESSION: Table '%s' changed from %s to %s",
							reg.Table, reg.OldScan, reg.NewScan)
					} else {
						criticalMsg = fmt.Sprintf(" - PLAN REGRESSION: %s", reg.Message)
					}
					break
				}
			}

			if hasCriticalRegression {
				fmt.Fprintf(w, "::error::Cost regression in %s: Expected %.2f, got %.2f (+%.1f%%)%s\n",
					r.Name, r.ExpectedCost, r.ActualCost, r.PercentIncrease, criticalMsg)
			} else {
				fmt.Fprintf(w, "::error::Cost regression in %s: Expected %.2f, got %.2f (+%.1f%%)\n",
					r.Name, r.ExpectedCost, r.ActualCost, r.PercentIncrease)
			}
		} else if r.Type == "output" {
			// Use structured diff for better error message if available
			if r.StructuredDiff != nil {
				sd := r.StructuredDiff
				var msg string
				switch sd.Type {
				case DiffTypeOrdering:
					msg = fmt.Sprintf("Output mismatch in %s: Same data (%d rows), different order", r.Name, sd.ExpectedRows)
				case DiffTypeRowCount:
					if sd.RemovedRows > 0 {
						msg = fmt.Sprintf("Output mismatch in %s: %d rows removed (expected %d, got %d)",
							r.Name, sd.RemovedRows, sd.ExpectedRows, sd.ActualRows)
					} else {
						msg = fmt.Sprintf("Output mismatch in %s: %d rows added (expected %d, got %d)",
							r.Name, sd.AddedRows, sd.ExpectedRows, sd.ActualRows)
					}
				case DiffTypeValues:
					msg = fmt.Sprintf("Output mismatch in %s: %d rows differ (out of %d)",
						r.Name, sd.ModifiedRows, sd.ExpectedRows)
				case DiffTypeMultiple:
					msg = fmt.Sprintf("Output mismatch in %s: %d added, %d removed, %d matching",
						r.Name, sd.AddedRows, sd.RemovedRows, sd.MatchingRows)
				default:
					msg = fmt.Sprintf("Output mismatch in %s", r.Name)
				}
				msg = strings.ReplaceAll(msg, "%", "%%")
				fmt.Fprintf(w, "::error::%s\n", msg)
			} else {
				// Fall back to generic message
				fmt.Fprintf(w, "::error::Output mismatch in %s\n", r.Name)
			}
		}
		if r.Error != "" {
			fmt.Fprintf(w, "::error::%s: %s\n", r.Name, r.Error)
		}
	case "warning":
		// Show plan quality warnings
		if len(r.PlanWarnings) > 0 {
			for _, warning := range r.PlanWarnings {
				msg := strings.ReplaceAll(warning.Message, "%", "%%")
				fmt.Fprintf(w, "::warning::%s - %s\n", r.Name, msg)
			}
		}
	case "skipped":
		fmt.Fprintf(w, "::warning::%s skipped: %s\n", r.Name, r.Error)
	}
	return nil
}

func (f *GitHubActionsFormatter) Finish(s *TestSummary, w io.Writer) error {
	fmt.Fprintln(w, "::endgroup::")

	if s.Failed > 0 {
		fmt.Fprintf(w, "::error::Test run failed: %d passed, %d failed", s.Passed, s.Failed)
		if s.Skipped > 0 {
			fmt.Fprintf(w, ", %d skipped", s.Skipped)
		}
		fmt.Fprintf(w, " (%.2fs)\n", s.Duration)
	} else {
		fmt.Fprintf(w, "::notice::All tests passed: %d passed", s.Passed)
		if s.Skipped > 0 {
			fmt.Fprintf(w, ", %d skipped", s.Skipped)
		}
		fmt.Fprintf(w, " (%.2fs)\n", s.Duration)
	}

	return nil
}

func init() {
	RegisterFormatter("github-actions", &GitHubActionsFormatter{})
}
