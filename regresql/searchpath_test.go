package regresql

import "testing"

// The extractor emits a comma-separated search_path. It cannot ride inside the
// `regresql:` option value, which is itself comma-split -- hence its own key.
func TestSearchPathAcceptsExtractorOutput(t *testing.T) {
	ok := []string{
		"regress_join_5, regress_join_4, regress_join_0, public",
		"public",
		"regress_join_0,public",
	}
	for _, sp := range ok {
		if !searchPathRx.MatchString(sp) {
			t.Errorf("rejected a search_path the extractor emits: %q", sp)
		}
	}
	// Anything that could smuggle a second statement must be refused rather
	// than interpolated into SET search_path = ...
	bad := []string{
		"public; DROP TABLE t",
		"public'",
		"$(whoami)",
		"",
		"1abc",
	}
	for _, sp := range bad {
		if sp != "" && searchPathRx.MatchString(sp) {
			t.Errorf("accepted a malformed search_path: %q", sp)
		}
	}
}

// admitPrelude opens with RESET ALL, so a search_path applied BEFORE it would
// be silently discarded and the query would resolve against the wrong schema --
// the exact failure this mechanism exists to prevent.
func TestAdmitPreludeStartsWithResetAll(t *testing.T) {
	if len(admitPrelude) == 0 || admitPrelude[0] != "RESET ALL" {
		t.Fatalf("admitPrelude no longer starts with RESET ALL (%v); the "+
			"search_path in canonicalResultHash must still be applied after it",
			admitPrelude)
	}
}
