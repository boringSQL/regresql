package regresql

import (
	"fmt"
	"io"
	"strings"
)

type ConsoleFormatter struct{}

func (f *ConsoleFormatter) Start(w io.Writer) error {
	fmt.Fprintln(w, "\nRunning regression tests...\n")
	return nil
}

func (f *ConsoleFormatter) AddResult(r TestResult, w io.Writer) error {
	switch r.Status {
	case "passed":
		fmt.Fprintf(w, "✓ %s (%.2fs)\n", r.Name, r.Duration)
	case "failed":
		fmt.Fprintf(w, "✗ %s (%.2fs)\n", r.Name, r.Duration)
		if r.Type == "cost" {
			fmt.Fprintf(w, "  Expected cost: %.2f\n", r.ExpectedCost)
			fmt.Fprintf(w, "  Actual cost:   %.2f\n", r.ActualCost)
			fmt.Fprintln(w)
			fmt.Fprintln(w, "  Likely cause: Missing index or outdated statistics")
		} else if r.Type == "output" && r.Diff != "" {
			lines := strings.Split(r.Diff, "\n")
			shown := 0
			for _, line := range lines {
				if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
					if shown < 5 {
						fmt.Fprintf(w, "  %s\n", line)
						shown++
					}
				} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
					if shown < 5 {
						fmt.Fprintf(w, "  %s\n", line)
						shown++
					}
				}
			}
			if shown >= 5 {
				fmt.Fprintln(w, "  ...")
			}
		}
		if r.Error != "" {
			fmt.Fprintf(w, "  Error: %s\n", r.Error)
		}
		fmt.Fprintln(w)
	case "skipped":
		// Don't show skipped in console
		return nil
	}
	return nil
}

func (f *ConsoleFormatter) Finish(s *TestSummary, w io.Writer) error {
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

func init() {
	RegisterFormatter("console", &ConsoleFormatter{})
}
