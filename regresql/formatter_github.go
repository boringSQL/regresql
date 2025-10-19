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
		// GitHub Actions shows passed tests as notice (optional)
		// We'll keep it quiet for passed tests
		return nil
	case "failed":
		if r.Type == "cost" {
			fmt.Fprintf(w, "::error::Cost regression in %s: Expected %.2f, got %.2f (+%.1f%%)\n",
				r.Name, r.ExpectedCost, r.ActualCost, r.PercentIncrease)
		} else if r.Type == "output" {
			// Escape newlines and percent signs for GitHub Actions
			diff := strings.ReplaceAll(r.Diff, "%", "%%")
			diff = strings.ReplaceAll(diff, "\n", "%0A")
			fmt.Fprintf(w, "::error::Output mismatch in %s\n", r.Name)
		}
		if r.Error != "" {
			fmt.Fprintf(w, "::error::%s: %s\n", r.Name, r.Error)
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
