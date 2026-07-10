package regresql

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// setsFor returns the SET overrides of a named perturbation, so a test can make a
// fake hash diverge under one specific plan without hard-coding its GUC strings.
func setsFor(name string) []string {
	for _, p := range admitPerturbations {
		if p.name == name {
			return p.sets
		}
	}
	return nil
}

// A query whose result never changes across every perturbation is admitted.
func TestAdmitDecision_Invariant(t *testing.T) {
	ok, reason := admitDecision(3, func([]string) (string, error) { return "same", nil })
	if !ok || reason != "" {
		t.Errorf("admitDecision = (%v, %q), want (true, \"\")", ok, reason)
	}
}

// If the result multiset changes under a perturbation, the query is rejected and
// the reason names the offending perturbation.
func TestAdmitDecision_RejectsNonInvariant(t *testing.T) {
	noSeqscan := setsFor("no_seqscan")
	hash := func(sets []string) (string, error) {
		if len(sets) > 0 && sets[0] == noSeqscan[0] {
			return "different", nil // this plan returns other rows
		}
		return "baseline", nil
	}

	ok, reason := admitDecision(3, hash)
	if ok {
		t.Fatal("admitDecision = admitted, want rejected")
	}
	if !strings.Contains(reason, "no_seqscan") {
		t.Errorf("reason = %q, want it to name no_seqscan", reason)
	}
}

// The "baseline" perturbation re-runs the unperturbed query; a result that flips
// on its 2nd rep is only sampled because reps > 1 — a single rep would miss it.
func TestAdmitDecision_RepsCatchRunToRun(t *testing.T) {
	calls := 0
	hash := func([]string) (string, error) {
		calls++
		if calls == 3 { // base=1, baseline rep1=2, baseline rep2=3 -> flip
			return "flip", nil
		}
		return "steady", nil
	}

	ok, reason := admitDecision(2, hash)
	if ok {
		t.Fatal("admitDecision = admitted, want rejected (run-to-run flip on rep 2)")
	}
	if !strings.Contains(reason, "baseline") {
		t.Errorf("reason = %q, want it to name the baseline perturbation", reason)
	}
}

// A hashing error on the baseline aborts with a baseline-specific reason.
func TestAdmitDecision_BaselineError(t *testing.T) {
	ok, reason := admitDecision(3, func([]string) (string, error) {
		return "", errors.New("connection refused")
	})
	if ok || !strings.HasPrefix(reason, "baseline error:") {
		t.Errorf("= (%v, %q), want rejected with baseline error", ok, reason)
	}
}

// A real (non-timeout) hashing error under a perturbation is reported against
// that perturbation, not swallowed as an admission.
func TestAdmitDecision_PerturbationError(t *testing.T) {
	ok, reason := admitDecision(1, func(sets []string) (string, error) {
		if len(sets) > 0 {
			return "", errors.New("out of memory")
		}
		return "base", nil
	})
	if ok {
		t.Fatal("admitDecision = admitted, want rejected on perturbation error")
	}
	if !strings.Contains(reason, "error") {
		t.Errorf("reason = %q, want an error reason", reason)
	}
}

// A perturbation that times out (a forced-bad plan too slow to run) is skipped,
// not counted as nondeterminism — the query is still admitted on the rest. This
// is the fix for the false-reject seen at 30M rows under no_hashagg.
func TestAdmitDecision_TimeoutSkipped(t *testing.T) {
	slow := setsFor("no_hashjoin")
	hash := func(sets []string) (string, error) {
		if len(sets) > 0 && sets[0] == slow[0] {
			return "", &pgconn.PgError{Code: "57014"} // statement_timeout cancel
		}
		return "same", nil
	}

	ok, reason := admitDecision(2, hash)
	if !ok {
		t.Errorf("admitDecision = rejected (%q), want admitted despite the timeout", reason)
	}
}

// renderAdmit's JSON form carries the admitted count, total, and per-query rows —
// the shape a CI gate reads.
func TestRenderAdmit_JSON(t *testing.T) {
	results := []AdmitResult{
		{Name: "q_ok", Admitted: true},
		{Name: "q_bad", Reason: "result not invariant under parallel"},
	}
	path := t.TempDir() + "/out.json"
	if err := renderAdmit(results, "json", path); err != nil {
		t.Fatalf("renderAdmit json: %v", err)
	}

	var got struct {
		Admitted int           `json:"admitted"`
		Total    int           `json:"total"`
		Results  []AdmitResult `json:"results"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Admitted != 1 || got.Total != 2 || len(got.Results) != 2 {
		t.Errorf("admitted/total/results = %d/%d/%d, want 1/2/2", got.Admitted, got.Total, len(got.Results))
	}
}

// renderAdmit's console form shows an ADMIT/REJECT line per result plus the tally.
func TestRenderAdmit_Console(t *testing.T) {
	results := []AdmitResult{
		{Name: "q_ok", Admitted: true},
		{Name: "q_bad", Reason: "result not invariant under parallel"},
	}
	path := t.TempDir() + "/out.txt"
	if err := renderAdmit(results, "console", path); err != nil {
		t.Fatalf("renderAdmit console: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out := string(data)
	for _, want := range []string{"ADMIT", "q_ok", "REJECT", "q_bad", "admitted 1 / 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("console output missing %q:\n%s", want, out)
		}
	}
}

// admitOneline collapses newlines and truncates long errors so a reason stays a
// single tidy line.
func TestAdmitOneline(t *testing.T) {
	if got := admitOneline("line1\nline2"); strings.Contains(got, "\n") {
		t.Errorf("admitOneline kept a newline: %q", got)
	}
	long := strings.Repeat("x", 200)
	if got := admitOneline(long); len([]rune(got)) > 91 { // 90 + ellipsis
		t.Errorf("admitOneline len = %d runes, want <= 91", len([]rune(got)))
	}
}
