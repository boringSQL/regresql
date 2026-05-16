package regresql

import (
	"testing"
)

// rs is a small helper to build a ResultSet inline. Keeps the table-driven
// cases below from drowning in boilerplate.
func rs(cols []string, rows [][]any) *ResultSet {
	return &ResultSet{Cols: cols, Rows: rows}
}

// TestCompareResultSets_IgnoreColumns covers the IgnoreColumns projection:
//   - a non-deterministic column (e.g. a timestamp) is dropped before
//     comparison so the remaining columns can be asserted equal
//   - ignore-listing a column that doesn't exist is a silent no-op (callers
//     may not know which columns are present)
//   - dropping the column that was the only source of difference flips a
//     would-be DiffTypeValues result into DiffTypeIdentical
func TestCompareResultSets_IgnoreColumns(t *testing.T) {
	tests := []struct {
		name     string
		expected *ResultSet
		actual   *ResultSet
		config   *DiffConfig
		wantType DiffType
		wantOK   bool
		wantCols []string
	}{
		{
			name: "drops non-deterministic column, otherwise identical",
			expected: rs(
				[]string{"id", "name", "created_at"},
				[][]any{
					{1, "alice", "2026-01-01T00:00:00Z"},
					{2, "bob", "2026-01-02T00:00:00Z"},
				},
			),
			actual: rs(
				[]string{"id", "name", "created_at"},
				[][]any{
					{1, "alice", "2026-05-16T12:00:00Z"},
					{2, "bob", "2026-05-16T12:00:01Z"},
				},
			),
			config:   &DiffConfig{MaxSamples: 5, IgnoreColumns: []string{"created_at"}},
			wantType: DiffTypeIdentical,
			wantOK:   true,
			wantCols: []string{"id", "name"},
		},
		{
			name: "ignore name that doesn't exist is a no-op",
			expected: rs(
				[]string{"id", "name"},
				[][]any{{1, "alice"}},
			),
			actual: rs(
				[]string{"id", "name"},
				[][]any{{1, "alice"}},
			),
			config:   &DiffConfig{MaxSamples: 5, IgnoreColumns: []string{"nope"}},
			wantType: DiffTypeIdentical,
			wantOK:   true,
			wantCols: []string{"id", "name"},
		},
		{
			name: "drops the column that was the only source of divergence",
			expected: rs(
				[]string{"id", "noise"},
				[][]any{{1, "a"}, {2, "b"}},
			),
			actual: rs(
				[]string{"id", "noise"},
				[][]any{{1, "x"}, {2, "y"}},
			),
			config:   &DiffConfig{MaxSamples: 5, IgnoreColumns: []string{"noise"}},
			wantType: DiffTypeIdentical,
			wantOK:   true,
			wantCols: []string{"id"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CompareResultSets(tc.expected, tc.actual, tc.config)
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tc.wantType)
			}
			if got.Identical != tc.wantOK {
				t.Errorf("Identical = %v, want %v", got.Identical, tc.wantOK)
			}
			if !equalStrings(got.Columns, tc.wantCols) {
				t.Errorf("Columns = %v, want %v", got.Columns, tc.wantCols)
			}
		})
	}
}

// TestCompareResultSets_IgnoreOrder pins the IgnoreOrder switch:
//   - default (false) keeps today's DiffTypeOrdering semantics
//   - true collapses an ordering-only difference to DiffTypeIdentical
func TestCompareResultSets_IgnoreOrder(t *testing.T) {
	expected := rs(
		[]string{"id"},
		[][]any{{1}, {2}, {3}},
	)
	actual := rs(
		[]string{"id"},
		[][]any{{3}, {1}, {2}},
	)

	t.Run("default reports DiffTypeOrdering", func(t *testing.T) {
		got := CompareResultSets(expected, actual, nil)
		if got.Type != DiffTypeOrdering {
			t.Errorf("Type = %q, want %q", got.Type, DiffTypeOrdering)
		}
		if got.Identical {
			t.Errorf("Identical = true, want false")
		}
	})

	t.Run("IgnoreOrder=true collapses to identical", func(t *testing.T) {
		cfg := &DiffConfig{MaxSamples: 5, IgnoreOrder: true}
		got := CompareResultSets(expected, actual, cfg)
		if got.Type != DiffTypeIdentical {
			t.Errorf("Type = %q, want %q", got.Type, DiffTypeIdentical)
		}
		if !got.Identical {
			t.Errorf("Identical = false, want true")
		}
		if got.MatchingRows != 3 {
			t.Errorf("MatchingRows = %d, want 3", got.MatchingRows)
		}
	})
}

// TestCompareResultSets_IgnoreColumnsAndOrder combines both flags: rows are
// permuted AND carry a non-deterministic column. With both flags set the
// comparison should report identical.
func TestCompareResultSets_IgnoreColumnsAndOrder(t *testing.T) {
	expected := rs(
		[]string{"id", "ts"},
		[][]any{
			{1, "2026-01-01T00:00:00Z"},
			{2, "2026-01-02T00:00:00Z"},
			{3, "2026-01-03T00:00:00Z"},
		},
	)
	actual := rs(
		[]string{"id", "ts"},
		[][]any{
			{3, "2026-05-01T00:00:00Z"},
			{1, "2026-05-02T00:00:00Z"},
			{2, "2026-05-03T00:00:00Z"},
		},
	)
	cfg := &DiffConfig{
		MaxSamples:    5,
		IgnoreColumns: []string{"ts"},
		IgnoreOrder:   true,
	}
	got := CompareResultSets(expected, actual, cfg)
	if got.Type != DiffTypeIdentical {
		t.Errorf("Type = %q, want %q", got.Type, DiffTypeIdentical)
	}
	if !got.Identical {
		t.Errorf("Identical = false, want true")
	}
}

// TestCompareResultSets_DefaultsUnchanged pins existing behavior with a
// zero-valued DiffConfig — additive flags must not alter prior semantics.
func TestCompareResultSets_DefaultsUnchanged(t *testing.T) {
	t.Run("identical case stays identical", func(t *testing.T) {
		a := rs([]string{"id"}, [][]any{{1}, {2}})
		b := rs([]string{"id"}, [][]any{{1}, {2}})
		got := CompareResultSets(a, b, nil)
		if got.Type != DiffTypeIdentical || !got.Identical {
			t.Errorf("got Type=%q Identical=%v, want identical", got.Type, got.Identical)
		}
	})

	t.Run("values-differ case stays DiffTypeValues", func(t *testing.T) {
		a := rs([]string{"id", "name"}, [][]any{{1, "alice"}, {2, "bob"}})
		b := rs([]string{"id", "name"}, [][]any{{1, "ALICE"}, {2, "BOB"}})
		got := CompareResultSets(a, b, nil)
		if got.Type != DiffTypeValues {
			t.Errorf("Type = %q, want %q", got.Type, DiffTypeValues)
		}
		if got.Identical {
			t.Errorf("Identical = true, want false")
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
