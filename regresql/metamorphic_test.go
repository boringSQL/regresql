package regresql

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// A query whose result survives every optimization flip is clean — no bug.
func TestMetamorphicDecision_Clean(t *testing.T) {
	bug, guc, reason := metamorphicDecision(func([]string) (string, error) { return "same", nil })
	if bug || guc != "" || reason != "" {
		t.Errorf("= (%v, %q, %q), want clean", bug, guc, reason)
	}
}

// If flipping one optimization off changes the result, that's a wrong-results
// bug and the offending GUC is named.
func TestMetamorphicDecision_FindsBug(t *testing.T) {
	hash := func(sets []string) (string, error) {
		if len(sets) > 0 && strings.Contains(sets[0], "enable_memoize") {
			return "different", nil // this optimization changed the rows
		}
		return "base", nil
	}

	bug, guc, _ := metamorphicDecision(hash)
	if !bug {
		t.Fatal("metamorphicDecision = clean, want a bug")
	}
	if guc != "enable_memoize" {
		t.Errorf("guc = %q, want enable_memoize", guc)
	}
}

// A GUC the server doesn't recognize (its SET errors) is skipped, and a real bug
// under a later optimization is still caught.
func TestMetamorphicDecision_SkipsUnknownGUC(t *testing.T) {
	firstOff := "SET " + metamorphicGUCs[0] + "=off"
	last := metamorphicGUCs[len(metamorphicGUCs)-1]
	hash := func(sets []string) (string, error) {
		switch {
		case len(sets) == 0:
			return "base", nil
		case sets[0] == firstOff:
			return "", errors.New("unrecognized configuration parameter")
		case strings.Contains(sets[0], last):
			return "moved", nil
		default:
			return "base", nil
		}
	}

	bug, guc, _ := metamorphicDecision(hash)
	if !bug || guc != last {
		t.Errorf("= (%v, %q), want a bug under %q despite the skipped first GUC", bug, guc, last)
	}
}

// A baseline hash failure aborts with a reason and no bug — there's nothing to
// compare the flips against.
func TestMetamorphicDecision_BaselineError(t *testing.T) {
	bug, _, reason := metamorphicDecision(func([]string) (string, error) {
		return "", errors.New("connection refused")
	})
	if bug || reason == "" {
		t.Errorf("= (bug=%v, reason=%q), want no bug with a reason", bug, reason)
	}
}

// renderMetamorphic's JSON form carries the bug count, total, and per-query rows —
// what a CI gate reads.
func TestRenderMetamorphic_JSON(t *testing.T) {
	results := []MetamorphicResult{
		{Name: "clean_q"},
		{Name: "buggy_q", Bug: true, GUC: "enable_eager_aggregate"},
	}
	path := t.TempDir() + "/m.json"
	renderMetamorphic(results, "json", path)

	var got struct {
		Bugs    int                 `json:"bugs"`
		Total   int                 `json:"total"`
		Results []MetamorphicResult `json:"results"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Bugs != 1 || got.Total != 2 || len(got.Results) != 2 {
		t.Errorf("bugs/total/results = %d/%d/%d, want 1/2/2", got.Bugs, got.Total, len(got.Results))
	}
}

// renderMetamorphic's console form flags the buggy query, names the GUC, and
// prints the tally.
func TestRenderMetamorphic_Console(t *testing.T) {
	results := []MetamorphicResult{
		{Name: "clean_q"},
		{Name: "buggy_q", Bug: true, GUC: "enable_eager_aggregate"},
	}
	path := t.TempDir() + "/m.txt"
	renderMetamorphic(results, "console", path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out := string(data)
	for _, want := range []string{"ok", "clean_q", "BUG", "buggy_q", "enable_eager_aggregate", "1 wrong-results bugs"} {
		if !strings.Contains(out, want) {
			t.Errorf("console output missing %q:\n%s", want, out)
		}
	}
}
