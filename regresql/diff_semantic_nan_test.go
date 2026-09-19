package regresql

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PostgreSQL defines NaN = NaN as TRUE (it needs a total order for btree), so
// two engines both returning NaN agree. Go's == disagrees, which turned every
// NaN-returning regress query into a false correctness break against itself.
//
// The values are float64 on purpose: tryToFloat64 accepts no string, so a
// "NaN" string never reaches the numeric branch this guards and would pass
// the test without exercising it at all.
func TestNaNComparesEqualLikePostgres(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	cases := []struct {
		name string
		a, b any
		want bool
	}{
		{"NaN vs NaN is equal (postgres semantics)", nan, nan, true},
		{"NaN vs a number is not", nan, 1.5, false},
		{"a number vs NaN is not", 1.5, nan, false},
		{"Inf vs Inf is equal", inf, inf, true},
		{"Inf vs -Inf is not", inf, math.Inf(-1), false},
		{"ordinary numbers still work", 1.5, 1.5, true},
		{"ordinary numbers still differ", 1.5, 1.6, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := valuesEqual(c.a, c.b, 0); got != c.want {
				t.Errorf("valuesEqual(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// The tolerance path is a separate return and needs its own guard: a NaN must
// not be swallowed by `|a-b| <= tolerance`, which is false for NaN either way
// and would report two NaNs as different.
func TestNonFiniteComparesEqualUnderFloatTolerance(t *testing.T) {
	if !valuesEqual(math.NaN(), math.NaN(), 0.001) {
		t.Error("NaN vs NaN under a float tolerance must still be equal")
	}
	if valuesEqual(math.NaN(), 1.5, 0.001) {
		t.Error("NaN vs a number under a float tolerance must not be equal")
	}
	// math.Abs(+Inf - +Inf) is NaN and NaN <= tolerance is false, so infinity
	// needs its own exit from the tolerance branch or it differs from itself.
	if !valuesEqual(math.Inf(1), math.Inf(1), 0.001) {
		t.Error("+Inf vs +Inf under a float tolerance must be equal")
	}
	if valuesEqual(math.Inf(1), math.Inf(-1), 0.001) {
		t.Error("+Inf vs -Inf must not be equal at any tolerance")
	}
	// Both sides resolve from the token, so this is the text-column case too.
	if !valuesEqual("Infinity", "Infinity", 0.001) {
		t.Error(`two identical "Infinity" values must not become a difference under tolerance`)
	}
}

// encoding/json refuses NaN and +/-Inf, so before ResultSet.MarshalJSON a
// query returning one could not have its expected file written at all --
// `update` failed on it and ToJSON silently returned "". The whole round trip
// has to work: write, load, compare equal.
func TestNonFiniteFloatsRoundTripThroughAnExpectedFile(t *testing.T) {
	live := &ResultSet{
		Cols: []string{"nan", "inf", "neginf", "ordinary", "text"},
		Rows: [][]any{{math.NaN(), math.Inf(1), math.Inf(-1), 1.5, "hello"}},
	}

	path := filepath.Join(t.TempDir(), "expected.json")
	if err := live.Write(path, true); err != nil {
		t.Fatalf("Write: %v", err) // was: json: unsupported value: NaN
	}

	// The documented format is the token itself -- readable and greppable in a
	// reviewed expected file. Assert the bytes, not just that it round-trips.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"NaN"`, `"Infinity"`, `"-Infinity"`, `1.5`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("expected file does not contain %s:\n%s", want, raw)
		}
	}

	// Marshalling must not rewrite the caller's rows -- the copy-on-write in
	// MarshalJSON is exactly what a later "optimization" removes.
	if f, ok := live.Rows[0][0].(float64); !ok || !math.IsNaN(f) {
		t.Errorf("Write mutated the caller's rows: [0][0] = %#v", live.Rows[0][0])
	}

	stored, err := LoadResultSet(path)
	if err != nil {
		t.Fatalf("LoadResultSet: %v", err)
	}
	if diff := CompareResultSets(stored, live, DefaultDiffConfig()); !diff.Identical {
		t.Errorf("stored result differs from the live one it was written from: %+v", diff)
	}
	// Comparison must not depend on which side is which.
	if diff := CompareResultSets(live, stored, DefaultDiffConfig()); !diff.Identical {
		t.Errorf("comparison is asymmetric: %+v", diff)
	}

	// `update` on an unchanged corpus must not rewrite the file.
	again := filepath.Join(filepath.Dir(path), "again.json")
	if err := stored.Write(again, true); err != nil {
		t.Fatalf("re-Write: %v", err)
	}
	raw2, err := os.ReadFile(again)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(raw2) {
		t.Errorf("stored tokens drift on rewrite:\n%s\n---\n%s", raw, raw2)
	}

	// The interactive-diff path marshals the same way; it must not fail.
	if _, err := live.ToJSON(); err != nil {
		t.Errorf("ToJSON on a non-finite result set: %v", err)
	}
}

// The tokens are recognised only as the exact strings PostgreSQL prints. A
// numeric string must NOT be parsed, or "1.50" and "1.5" would start comparing
// equal and mask real differences.
func TestOnlyNonFiniteTokensAreResolvedFromStrings(t *testing.T) {
	if valuesEqual("1.50", "1.5", 0) {
		t.Error(`"1.50" and "1.5" are different strings and must not compare equal`)
	}
	if !valuesEqual("NaN", math.NaN(), 0) {
		t.Error(`stored "NaN" must equal a live NaN`)
	}
	if !valuesEqual("Infinity", math.Inf(1), 0) {
		t.Error(`stored "Infinity" must equal a live +Inf`)
	}
	if valuesEqual("Infinity", math.Inf(-1), 0) {
		t.Error(`stored "Infinity" must not equal -Inf`)
	}
	if valuesEqual("NaN", "hello", 0) {
		t.Error(`"NaN" must not equal an unrelated string`)
	}
}

// Only the exact spellings PostgreSQL prints are tokens. "inf" is an input
// form the server accepts but never emits, so treating it as one would make two
// genuinely different text values compare equal -- masking a difference on the
// gated tier, the opposite of the bug this fixes.
func TestTokensAreMatchedExactly(t *testing.T) {
	for _, c := range []struct{ a, b any }{
		{"inf", "Infinity"},
		{"inf", math.Inf(1)},
		{"infinity", math.Inf(1)},
		{"nan", math.NaN()},
		{" NaN ", "NaN"},
		{"NAN", math.NaN()},
	} {
		if valuesEqual(c.a, c.b, 0) {
			t.Errorf("valuesEqual(%q, %v) = true; only the exact token is a token", c.a, c.b)
		}
	}
}
