package regresql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// scoreboardTotals is the cover-letter summary the whole feature exists to emit.
type scoreboardTotals struct {
	Queries     int
	Correctness int
	Shape       int

	CriticalPlan     int
	QErrImproved     int
	QErrRegressed    int
	BufferRegress    int
	Spill            int
	Incomplete       int
	Errors           int
	Excluded         int
	CostTie          int
	CostTieAnnotated int
	SelfFloored      int
	TimingSlower     int
	TimingFaster     int
	TimingUnstable   int
}

func (b *Scoreboard) totals() scoreboardTotals {
	t := scoreboardTotals{
		Queries: len(b.Comparisons), Excluded: len(b.Excluded), CostTie: len(b.CostTie),
		CostTieAnnotated: b.CostTieAnnotated, SelfFloored: b.SelfFloored,
	}
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
		if c.bufferRegressed() {
			t.BufferRegress++
		}
		if c.CriticalPlan {
			t.CriticalPlan++
		}
		if c.QErrorWorse {
			t.QErrRegressed++
		} else if c.BaseQError > 0 && c.TargetQError > 0 && c.TargetQError < c.BaseQError {
			t.QErrImproved++
		}
		if c.Timing != nil {
			switch c.Timing.Status {
			case "slower":
				t.TimingSlower++
			case "faster":
				t.TimingFaster++
			case "unstable":
				t.TimingUnstable++
			}
		}
	}
	return t
}

// line renders the headline with the GATED tiers first and the advisory ones
// behind an explicit marker. Correctness, buffers and spill are gated and drive
// the exit code; shape and q-error are advisory (see renderMarkdown).
func (t scoreboardTotals) line() string {
	s := fmt.Sprintf(
		"%d queries · %d correctness · %d buffer · %d spill · %d incomplete",
		t.Queries, t.Correctness, t.BufferRegress, t.Spill, t.Incomplete)
	if t.Excluded > 0 {
		s += fmt.Sprintf(" · %d excluded", t.Excluded)
	}
	if t.CostTie > 0 {
		s += fmt.Sprintf(" · %d cost-tie (excluded)", t.CostTie)
	}
	// spelled out so an annotated (still compared) tie is never read as excluded
	if t.CostTieAnnotated > 0 {
		s += fmt.Sprintf(" · %d cost-tie (compared, annotated)", t.CostTieAnnotated)
	}
	if t.SelfFloored > 0 {
		s += fmt.Sprintf(" · %d self-control-floored", t.SelfFloored)
	}
	adv := fmt.Sprintf("%d shape (%d notable) · q-error +%d/-%d",
		t.Shape, t.CriticalPlan, t.QErrImproved, t.QErrRegressed)
	if t.TimingSlower+t.TimingFaster+t.TimingUnstable > 0 {
		adv += fmt.Sprintf(" · timing %d slower / %d faster (%d unstable)",
			t.TimingSlower, t.TimingFaster, t.TimingUnstable)
	}
	return s + "  ||  advisory: " + adv
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
	b.Generated = time.Now().UTC().Format(time.RFC3339)
	// render fully before writing so a failed render can't leave a stale file
	var buf bytes.Buffer
	var err error
	switch format {
	case "json":
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		err = enc.Encode(b)
	case "markdown":
		err = b.renderMarkdown(&buf)
	default:
		err = b.renderConsole(&buf)
	}
	if err != nil {
		return err
	}
	if outputPath == "" {
		_, err = os.Stdout.Write(buf.Bytes())
		return err
	}
	tmp := outputPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, outputPath) // atomic: no partial file survives a crash
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
	if c.bufferRegressed() {
		if c.BaseBuffers == 0 {
			parts = append(parts, fmt.Sprintf("buffers 0→%d blocks", c.TargetBuffers))
		} else {
			parts = append(parts, fmt.Sprintf("buffers +%.1f%%", c.BufferDelta))
		}
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
	if c.Timing != nil {
		switch c.Timing.Status {
		case "slower":
			parts = append(parts, fmt.Sprintf("%.2fx slower", c.Timing.Ratio))
		case "faster":
			parts = append(parts, fmt.Sprintf("%.2fx faster", 1/c.Timing.Ratio))
		case "unstable":
			parts = append(parts, "timing unstable")
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
	fmt.Fprintf(w, "regresql compare  (generated %s)\n", b.Generated)
	fmt.Fprintf(w, "  base:   %s (%d)\n", b.Base.Version, b.Base.VersionNum)
	fmt.Fprintf(w, "  target: %s (%d)\n", b.Target.Version, b.Target.VersionNum)
	fmt.Fprintf(w, "  %s\n", b.costLine())
	if b.StatsInjected {
		fmt.Fprintln(w, "  statistics: injected (identical on both — diffs are planner code, not ANALYZE noise)")
		fmt.Fprintln(w, "  CAVEAT: base's statistics OVERWRITE target's, so a change to what ANALYZE")
		fmt.Fprintln(w, "          *produces* is erased before comparison. A patch touching statistics")
		fmt.Fprintln(w, "          collection is NOT TESTED here — rerun without --inject-stats (keep")
		fmt.Fprintln(w, "          --self-control, which then measures the ANALYZE-sampling floor).")
	}
	for _, g := range b.GUCMismatch {
		fmt.Fprintf(w, "  GUC mismatch: %s base=%s target=%s\n", g.Name, g.Base, g.Target)
	}
	fmt.Fprintln(w)

	for _, c := range b.sortedComparisons() {
		icon := severityIcon[c.Severity]
		detail := compareDetail(c)
		fmt.Fprintf(w, "  %s  %-28s %s\n", icon, compareLabel(c), detail)
	}

	for _, e := range b.Excluded {
		fmt.Fprintf(w, "  ⊘ EXCL   %-28s %s\n", admitLabel(e), e.Reason)
	}
	for _, e := range b.CostTie {
		fmt.Fprintf(w, "  ⊘ TIE    %-28s %s\n", admitLabel(e), e.Reason)
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
	fmt.Fprintf(w, "_generated %s_\n\n", b.Generated)
	fmt.Fprintf(w, "**%s**\n\n", t.line())
	fmt.Fprintf(w, "_Gated tiers (correctness, buffer, spill, incomplete) drive the verdict and the\n"+
		"exit code. Advisory tiers are reported but not gated: a plan-shape flip on a\n"+
		"cost-tie is not yet separable from a real one without a cost-margin classifier,\n"+
		"and the q-error improved counter has no calibration floor — it reads nonzero\n"+
		"comparing a build to itself. Do not quote advisory numbers as a patch verdict._\n\n")
	fmt.Fprintf(w, "%s\n\n", b.costLine())
	if b.StatsInjected {
		fmt.Fprintf(w, "_statistics injected (identical on both — differences are planner code, not ANALYZE noise)_\n\n")
		fmt.Fprintf(w, "> **Caveat — collection-side changes are invisible in this run.** Base's\n"+
			"> statistics overwrite target's, so a patch that changes what `ANALYZE`\n"+
			"> *produces* (new or better statistics) has its output erased before the\n"+
			"> comparison runs. For such a patch this scoreboard means **not tested**,\n"+
			"> not \"no change\". Rerun without `--inject-stats` and with\n"+
			"> `--self-control`, which then calibrates the ANALYZE-sampling floor.\n\n")
	}

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
	for _, e := range b.Excluded {
		fmt.Fprintf(w, "| excluded | `%s` | %s |\n", admitLabel(e), e.Reason)
	}
	for _, e := range b.CostTie {
		fmt.Fprintf(w, "| cost-tie | `%s` | %s |\n", admitLabel(e), e.Reason)
	}
	return nil
}

func admitLabel(e AdmitResult) string {
	if e.Binding != "" {
		return e.Name + " (" + e.Binding + ")"
	}
	return e.Name
}
