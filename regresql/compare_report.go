package regresql

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// scoreboardTotals is the cover-letter summary the whole feature exists to emit.
type scoreboardTotals struct {
	Queries       int
	Correctness   int
	Shape         int
	QErrImproved  int
	QErrRegressed int
	BufferRegress int
	Spill         int
	Incomplete    int
	Errors        int
}

func (b *Scoreboard) totals() scoreboardTotals {
	t := scoreboardTotals{Queries: len(b.Comparisons)}
	for _, c := range b.Comparisons {
		switch {
		case c.Severity == SevError:
			t.Errors++
		case c.Severity == SevIncomplete:
			t.Incomplete++
		}
		if c.ResultDiffer {
			t.Correctness++
		}
		if c.PlanChanged {
			t.Shape++
		}
		if c.SpillRegress {
			t.Spill++
		}
		if c.BufferDelta > GetBufferThreshold() {
			t.BufferRegress++
		}
		if c.QErrorWorse {
			t.QErrRegressed++
		} else if c.BaseQError > 0 && c.TargetQError > 0 && c.TargetQError < c.BaseQError {
			t.QErrImproved++
		}
	}
	return t
}

func (t scoreboardTotals) line() string {
	return fmt.Sprintf(
		"%d queries · %d correctness · %d shape · q-error +%d/-%d · %d buffer · %d spill · %d incomplete",
		t.Queries, t.Correctness, t.Shape, t.QErrImproved, t.QErrRegressed,
		t.BufferRegress, t.Spill, t.Incomplete)
}

func (b *Scoreboard) costLine() string {
	if b.SameVersion {
		return "cost: comparable (same server version)"
	}
	return fmt.Sprintf("cost: suppressed (server versions differ: %d vs %d)",
		b.Base.VersionNum, b.Target.VersionNum)
}

// sortedComparisons orders worst-first so the interesting rows lead.
func (b *Scoreboard) sortedComparisons() []QueryComparison {
	out := make([]QueryComparison, len(b.Comparisons))
	copy(out, b.Comparisons)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Severity > out[j].Severity
	})
	return out
}

func renderScoreboard(b *Scoreboard, format, outputPath string) error {
	w, closeFn, err := getWriter(outputPath)
	if err != nil {
		return err
	}
	defer closeFn()

	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(b)
	case "markdown":
		return b.renderMarkdown(w)
	default:
		return b.renderConsole(w)
	}
}

func compareLabel(c QueryComparison) string {
	if c.Binding != "" {
		return c.Name + " (" + c.Binding + ")"
	}
	return c.Name
}

func compareDetail(c QueryComparison) string {
	var parts []string
	if c.Note != "" {
		parts = append(parts, c.Note)
	}
	if c.ResultDiffer {
		parts = append(parts, "rows differ")
	}
	if c.SpillRegress {
		parts = append(parts, "spill")
	}
	if c.BufferDelta > GetBufferThreshold() {
		parts = append(parts, fmt.Sprintf("buffers +%.1f%%", c.BufferDelta))
	}
	if c.QErrorWorse {
		parts = append(parts, fmt.Sprintf("q-error %.0fx→%.0fx (%s)", c.BaseQError, c.TargetQError, c.QErrorNode))
	}
	if c.PlanChanged {
		for _, r := range c.Regressions {
			parts = append(parts, r.Message)
		}
		if len(c.Regressions) == 0 {
			parts = append(parts, "plan changed")
		}
	}
	return strings.Join(parts, ", ")
}

var severityIcon = map[Severity]string{
	SevEqual:       "=  ok  ",
	SevShape:       "Δ SHAPE",
	SevEstimation:  "~ ESTIM",
	SevPerf:        "⚠ PERF ",
	SevIncomplete:  "… INCMP",
	SevCorrectness: "✗ WRONG",
	SevError:       "! ERROR",
}

func (b *Scoreboard) renderConsole(w io.Writer) error {
	fmt.Fprintln(w, "regresql compare")
	fmt.Fprintf(w, "  base:   %s (%d)\n", b.Base.Version, b.Base.VersionNum)
	fmt.Fprintf(w, "  target: %s (%d)\n", b.Target.Version, b.Target.VersionNum)
	fmt.Fprintf(w, "  %s\n", b.costLine())
	for _, g := range b.GUCMismatch {
		fmt.Fprintf(w, "  GUC mismatch: %s base=%s target=%s\n", g.Name, g.Base, g.Target)
	}
	fmt.Fprintln(w)

	for _, c := range b.sortedComparisons() {
		icon := severityIcon[c.Severity]
		detail := compareDetail(c)
		fmt.Fprintf(w, "  %s  %-28s %s\n", icon, compareLabel(c), detail)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "  ── scoreboard ──")
	fmt.Fprintf(w, "  %s\n", b.totals().line())
	fmt.Fprintf(w, "  %s\n", b.costLine())
	return nil
}

func (b *Scoreboard) renderMarkdown(w io.Writer) error {
	t := b.totals()
	fmt.Fprintf(w, "## regresql compare: `%s` → `%s`\n\n", b.Base.Version, b.Target.Version)
	fmt.Fprintf(w, "**%s**\n\n", t.line())
	fmt.Fprintf(w, "%s\n\n", b.costLine())

	if len(b.GUCMismatch) > 0 {
		fmt.Fprintln(w, "> GUC mismatches (comparison may be unfair):")
		for _, g := range b.GUCMismatch {
			fmt.Fprintf(w, "> - `%s`: base `%s` vs target `%s`\n", g.Name, g.Base, g.Target)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "| severity | query | detail |")
	fmt.Fprintln(w, "|---|---|---|")
	for _, c := range b.sortedComparisons() {
		detail := compareDetail(c)
		if detail == "" {
			detail = "—"
		}
		fmt.Fprintf(w, "| %s | `%s` | %s |\n", c.Severity, compareLabel(c), detail)
	}
	return nil
}
