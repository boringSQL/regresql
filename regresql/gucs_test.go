package regresql

import (
	"strings"
	"testing"

	"github.com/boringsql/queries"
)

// Metadata is attached by the file scanner, not by NewQueryFromString, so the
// `-- gucs:` line has to be handed over as the parsed map the scanner produces.
func gucQuery(t *testing.T, gucs string) *Query {
	t.Helper()
	md := map[string]string{}
	if gucs != "" {
		md["gucs"] = gucs
	}
	bq, err := queries.NewQuery("t", "t.sql", "SELECT 1", md)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}
	return &Query{Query: bq}
}

// The shapes the extractor actually emits, taken verbatim from the regenerated
// corpus. memoize_021 is the cf/7175 test: without these GUCs no Memoize plan
// forms, the wrong-results bug is unreachable, and the harness reports silence.
func TestGetGUCsAcceptsExtractorOutput(t *testing.T) {
	q := gucQuery(t, "enable_bitmapscan=off; enable_hashjoin=off; "+
		"enable_material=off; enable_mergejoin=off; enable_seqscan=off; "+
		"hash_mem_multiplier=1.0; work_mem='64kB'")
	got := q.GetGUCs()
	if len(got) != 7 {
		t.Fatalf("parsed %d GUCs, want 7: %+v", len(got), got)
	}
	if got[0].Name != "enable_bitmapscan" || got[0].Value != "off" {
		t.Errorf("first GUC = %+v", got[0])
	}
	if got[6].Name != "work_mem" || got[6].Value != "'64kB'" {
		t.Errorf("quoted value mangled: %+v", got[6])
	}
}

// DateStyle is 'European,ISO' -- a comma INSIDE the value. That is why the
// separator is ';' and why this key cannot be a `regresql:` sub-option.
func TestGetGUCsKeepsCommasInsideValues(t *testing.T) {
	q := gucQuery(t, "datestyle='European,ISO'; enable_seqscan=off")
	got := q.GetGUCs()
	if len(got) != 2 {
		t.Fatalf("comma inside a quoted value split the list: %+v", got)
	}
	if got[0].Value != "'European,ISO'" {
		t.Errorf("value = %q, want 'European,ISO'", got[0].Value)
	}
}

// These strings reach us from a machine extractor reading files we do not
// control, and they are concatenated into a SET. Anything that could carry a
// second statement must be dropped, not interpolated.
func TestGetGUCsRefusesInjection(t *testing.T) {
	bad := []string{
		"work_mem=1; DROP TABLE t",       // ';' splits, so the tail parses as its own pair
		"enable_seqscan=off) ; select 1", // stray punctuation
		"1abc=off",                       // not an identifier
		"enable_seqscan=$(whoami)",
		"enable_seqscan=off\nDROP TABLE t",
		"a b=off",
	}
	for _, s := range bad {
		q := gucQuery(t, s)
		for _, g := range q.GetGUCs() {
			if !gucNameRx.MatchString(g.Name) || !gucValueRx.MatchString(g.Value) {
				t.Errorf("%q yielded an unvalidated GUC %+v", s, g)
			}
			if strings.ContainsAny(g.Name+g.Value, ";\n$()") {
				t.Errorf("%q yielded a GUC carrying shell/SQL punctuation: %+v", s, g)
			}
		}
	}
}

func TestGetGUCsAbsentIsNil(t *testing.T) {
	if g := gucQuery(t, "").GetGUCs(); g != nil {
		t.Errorf("no metadata should yield no GUCs, got %+v", g)
	}
	if g := gucQuery(t, " ").GetGUCs(); len(g) != 0 {
		t.Errorf("empty metadata should yield no GUCs, got %+v", g)
	}
}

// A dropped GUC means the query is NOT planned the way its source file
// intended. That has to reach the row; silently proceeding is the failure this
// whole mechanism exists to fix.
func TestGUCSkipNoteIsEmptyOnlyWhenNothingSkipped(t *testing.T) {
	if gucSkipNote(nil) != "" {
		t.Error("no skips should produce no note")
	}
	n := gucSkipNote([]string{"enable_groupagg", "jit_above_cost"})
	if !strings.Contains(n, "enable_groupagg") || !strings.Contains(n, "jit_above_cost") {
		t.Errorf("note does not name the skipped GUCs: %q", n)
	}
}

// assignment kept only the last observation, so a skipped GUC vanished as soon
// as the row was also a cost-tie
func TestAddNoteAppends(t *testing.T) {
	var c QueryComparison
	c.addNote("")
	c.addNote("first")
	c.addNote("")
	c.addNote("second")
	if c.Note != "first; second" {
		t.Errorf("Note = %q, want %q", c.Note, "first; second")
	}
}
