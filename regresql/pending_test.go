package regresql

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/boringsql/queries"
)

func TestTestSummaryAddResultPending(t *testing.T) {
	summary := NewTestSummary()

	// Add a pending result
	summary.AddResult(TestResult{
		Name:   "test1",
		Status: "pending",
	})

	if summary.Total != 1 {
		t.Errorf("Expected Total=1, got %d", summary.Total)
	}
	if summary.Pending != 1 {
		t.Errorf("Expected Pending=1, got %d", summary.Pending)
	}
	if summary.Passed != 0 {
		t.Errorf("Expected Passed=0, got %d", summary.Passed)
	}
	if summary.Failed != 0 {
		t.Errorf("Expected Failed=0, got %d", summary.Failed)
	}
}

func TestTestSummaryMixedStatuses(t *testing.T) {
	summary := NewTestSummary()

	summary.AddResult(TestResult{Name: "passed1", Status: "passed"})
	summary.AddResult(TestResult{Name: "pending1", Status: "pending"})
	summary.AddResult(TestResult{Name: "failed1", Status: "failed"})
	summary.AddResult(TestResult{Name: "pending2", Status: "pending"})
	summary.AddResult(TestResult{Name: "skipped1", Status: "skipped"})

	if summary.Total != 5 {
		t.Errorf("Expected Total=5, got %d", summary.Total)
	}
	if summary.Passed != 1 {
		t.Errorf("Expected Passed=1, got %d", summary.Passed)
	}
	if summary.Failed != 1 {
		t.Errorf("Expected Failed=1, got %d", summary.Failed)
	}
	if summary.Pending != 2 {
		t.Errorf("Expected Pending=2, got %d", summary.Pending)
	}
	if summary.Skipped != 1 {
		t.Errorf("Expected Skipped=1, got %d", summary.Skipped)
	}
}

func TestCompareResultSetsToResultsMissingExpectedFile(t *testing.T) {
	// Create temp directories
	tmpDir, err := os.MkdirTemp("", "regresql-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	regressDir := filepath.Join(tmpDir, "regress")
	outDir := filepath.Join(regressDir, "out")
	expectedDir := filepath.Join(regressDir, "expected")

	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("Failed to create out dir: %v", err)
	}
	if err := os.MkdirAll(expectedDir, 0755); err != nil {
		t.Fatalf("Failed to create expected dir: %v", err)
	}

	// Create an actual result file (simulating query output)
	actualFile := filepath.Join(outDir, "test_query.json")
	actualContent := `{"columns":["id"],"rows":[[1]]}`
	if err := os.WriteFile(actualFile, []byte(actualContent), 0644); err != nil {
		t.Fatalf("Failed to write actual file: %v", err)
	}

	// Create a minimal query for the plan
	bqQuery, _ := queries.NewQuery("test", "test.sql", "SELECT 1", nil)
	query := &Query{Query: bqQuery}

	// Create a plan with the result set (no expected file exists)
	plan := &Plan{
		Query: query,
		ResultSets: []ResultSet{
			{Filename: actualFile},
		},
		Names:    []string{"default"},
		Bindings: []map[string]any{{}},
	}

	// Run comparison - expected file doesn't exist
	results := plan.CompareResultSetsToResults(regressDir, expectedDir)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Status != "pending" {
		t.Errorf("Expected status='pending', got '%s'", result.Status)
	}
	if result.Error == "" {
		t.Error("Expected error message to be set for pending result")
	}
}

func TestCompareResultSetsToResultsWithExpectedFile(t *testing.T) {
	// Create temp directories
	tmpDir, err := os.MkdirTemp("", "regresql-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	regressDir := filepath.Join(tmpDir, "regress")
	outDir := filepath.Join(regressDir, "out")
	expectedDir := filepath.Join(regressDir, "expected")

	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("Failed to create out dir: %v", err)
	}
	if err := os.MkdirAll(expectedDir, 0755); err != nil {
		t.Fatalf("Failed to create expected dir: %v", err)
	}

	// Create matching actual and expected files
	content := `{"columns":["id"],"rows":[[1]]}`
	actualFile := filepath.Join(outDir, "test_query.json")
	expectedFile := filepath.Join(expectedDir, "test_query.json")

	if err := os.WriteFile(actualFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write actual file: %v", err)
	}
	if err := os.WriteFile(expectedFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write expected file: %v", err)
	}

	bqQuery2, _ := queries.NewQuery("test", "test.sql", "SELECT 1", nil)
	query := &Query{Query: bqQuery2}

	plan := &Plan{
		Query: query,
		ResultSets: []ResultSet{
			{
				Filename: actualFile,
				Cols:     []string{"id"},
				Rows:     [][]any{{float64(1)}}, // JSON unmarshals numbers as float64
			},
		},
		Names:    []string{"default"},
		Bindings: []map[string]any{{}},
	}

	results := plan.CompareResultSetsToResults(regressDir, expectedDir)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Status != "passed" {
		t.Errorf("Expected status='passed', got '%s' (error: %s)", result.Status, result.Error)
	}
}
