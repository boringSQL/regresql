package regresql

import (
	"encoding/json"
	"math"
	"reflect"
	"time"
)

type (
	DiffType string

	StructuredDiff struct {
		Type      DiffType
		Identical bool

		ExpectedRows int
		ActualRows   int
		MatchingRows int
		AddedRows    int
		RemovedRows  int
		ModifiedRows int

		AddedSamples    [][]any
		RemovedSamples  [][]any
		ModifiedSamples []RowDiff

		Columns []string
	}

	RowDiff struct {
		ExpectedRow []any
		ActualRow   []any
	}

	DiffConfig struct {
		FloatTolerance float64
		MaxSamples     int
	}
)

const (
	DiffTypeIdentical DiffType = "identical"
	DiffTypeRowCount  DiffType = "row_count"
	DiffTypeOrdering  DiffType = "ordering"
	DiffTypeValues    DiffType = "values"
	DiffTypeMultiple  DiffType = "multiple"
)

func DefaultDiffConfig() *DiffConfig {
	return &DiffConfig{MaxSamples: 5}
}

// CompareResultSets performs semantic comparison of two ResultSet objects
func CompareResultSets(expected, actual *ResultSet, config *DiffConfig) *StructuredDiff {
	if config == nil {
		config = DefaultDiffConfig()
	}

	diff := &StructuredDiff{
		Identical:    true,
		ExpectedRows: len(expected.Rows),
		ActualRows:   len(actual.Rows),
		Columns:      expected.Cols,
	}

	// Check column structure
	if !columnsMatch(expected.Cols, actual.Cols) {
		diff.Identical = false
		diff.Type = DiffTypeValues
		return diff
	}

	// Quick check: identical content in order
	if len(expected.Rows) == len(actual.Rows) {
		identical, modifiedIndices := compareRowsInOrder(expected, actual, config)
		if identical {
			diff.Type = DiffTypeIdentical
			diff.MatchingRows = len(expected.Rows)
			return diff
		}

		// Same count but values differ - check if just ordering
		if len(modifiedIndices) > 0 {
			// Try unordered matching to see if it's just ordering
			matchedExpected, _, unmatchedExpected, unmatchedActual := matchRowsUnordered(expected, actual, config)

			if len(unmatchedExpected) == 0 && len(unmatchedActual) == 0 {
				// All rows match when unordered - just ordering changed
				diff.Identical = false
				diff.Type = DiffTypeOrdering
				diff.MatchingRows = len(expected.Rows)
				return diff
			}

			// Some rows truly differ
			diff.Identical = false
			diff.Type = DiffTypeValues
			diff.MatchingRows = len(matchedExpected)
			diff.ModifiedRows = len(unmatchedExpected)

			// Collect samples
			diff.ModifiedSamples = collectModifiedSamples(expected, actual, unmatchedExpected, unmatchedActual, config.MaxSamples)

			return diff
		}
	}

	// Row counts differ - detailed analysis
	matchedExpected, _, unmatchedExpected, unmatchedActual := matchRowsUnordered(expected, actual, config)

	diff.Identical = false
	diff.MatchingRows = len(matchedExpected)
	diff.AddedRows = len(unmatchedActual)
	diff.RemovedRows = len(unmatchedExpected)

	if diff.AddedRows > 0 && diff.RemovedRows > 0 {
		diff.Type = DiffTypeMultiple
	} else {
		diff.Type = DiffTypeRowCount
	}

	// Collect samples
	diff.AddedSamples = collectSamples(actual, unmatchedActual, config.MaxSamples)
	diff.RemovedSamples = collectSamples(expected, unmatchedExpected, config.MaxSamples)

	return diff
}

// columnsMatch checks if two column lists are identical
func columnsMatch(expected, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}
	for i := range expected {
		if expected[i] != actual[i] {
			return false
		}
	}
	return true
}

func compareRowsInOrder(expected, actual *ResultSet, config *DiffConfig) (bool, []int) {
	var diffs []int
	for i := range expected.Rows {
		if !rowsEqual(expected.Rows[i], actual.Rows[i], config.FloatTolerance) {
			diffs = append(diffs, i)
		}
	}
	return len(diffs) == 0, diffs
}

// rowsEqual compares two rows for equality
func rowsEqual(expectedRow, actualRow []any, floatTolerance float64) bool {
	if len(expectedRow) != len(actualRow) {
		return false
	}

	for i := range expectedRow {
		if !valuesEqual(expectedRow[i], actualRow[i], floatTolerance) {
			return false
		}
	}

	return true
}

func valuesEqual(a, b any, floatTolerance float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Always try numeric comparison when both values are numeric types
	// This handles cases like int64 vs float64 regardless of tolerance setting
	if aNum, aOk := tryToFloat64(a); aOk {
		if bNum, bOk := tryToFloat64(b); bOk {
			if floatTolerance > 0 {
				return math.Abs(aNum-bNum) <= floatTolerance
			}
			return aNum == bNum
		}
	}

	// Try timestamp comparison
	if aTime, aOk := tryToTime(a); aOk {
		if bTime, bOk := tryToTime(b); bOk {
			return aTime.Equal(bTime)
		}
	}

	return reflect.DeepEqual(a, b)
}

// tryToFloat64 attempts to convert a value to float64
func tryToFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// tryToTime attempts to convert a value to time.Time
func tryToTime(v any) (time.Time, bool) {
	switch val := v.(type) {
	case time.Time:
		return val, true
	case string:
		return toTime(val)
	default:
		return time.Time{}, false
	}
}

// toTime attempts to parse a string as a timestamp in various formats
func toTime(s string) (time.Time, bool) {
	// Try RFC3339Nano first (most precise)
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func matchRowsUnordered(expected, actual *ResultSet, config *DiffConfig) (
	matchedExpected, matchedActual, unmatchedExpected, unmatchedActual []int) {

	used := make(map[int]bool, len(actual.Rows))

	for ei, expRow := range expected.Rows {
		found := false
		for ai, actRow := range actual.Rows {
			if used[ai] {
				continue
			}
			if rowsEqual(expRow, actRow, config.FloatTolerance) {
				matchedExpected = append(matchedExpected, ei)
				matchedActual = append(matchedActual, ai)
				used[ai] = true
				found = true
				break
			}
		}
		if !found {
			unmatchedExpected = append(unmatchedExpected, ei)
		}
	}

	for ai := range actual.Rows {
		if !used[ai] {
			unmatchedActual = append(unmatchedActual, ai)
		}
	}

	return
}

func collectSamples(rs *ResultSet, indices []int, maxSamples int) [][]any {
	var samples [][]any
	for i, idx := range indices {
		if i >= maxSamples {
			break
		}
		if idx >= 0 && idx < len(rs.Rows) {
			samples = append(samples, rs.Rows[idx])
		}
	}
	return samples
}

func collectModifiedSamples(expected, actual *ResultSet, unmatchedExpected, unmatchedActual []int, maxSamples int) []RowDiff {
	var samples []RowDiff
	n := len(unmatchedExpected)
	if len(unmatchedActual) < n {
		n = len(unmatchedActual)
	}
	if n > maxSamples {
		n = maxSamples
	}

	for i := 0; i < n; i++ {
		samples = append(samples, RowDiff{
			ExpectedRow: expected.Rows[unmatchedExpected[i]],
			ActualRow:   actual.Rows[unmatchedActual[i]],
		})
	}
	return samples
}
