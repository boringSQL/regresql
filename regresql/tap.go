package regresql

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mndrix/tap-go"
)

/*
CompareResultsSets load the expected result set and compares it with the
given Plan's ResultSet, and fills in a tap.T test output.

The test is considered passed when the diff is empty.

Rather than returning an error in case something wrong happens, we register
a diagnostic against the tap output and fail the test case.
*/
func (p *Plan) CompareResultSets(regressDir string, expectedDir string, t *tap.T) {
	for i, rs := range p.ResultSets {
		testName := strings.TrimPrefix(rs.Filename, regressDir+"/out/")
		expectedFilename := filepath.Join(expectedDir,
			filepath.Base(rs.Filename))
		diff, err := DiffFiles(expectedFilename, rs.Filename, 3)

		// Get binding info if available (for queries with parameters)
		var bindingName string
		var bindings map[string]string
		if i < len(p.Names) {
			bindingName = p.Names[i]
		} else {
			bindingName = "n/a"
		}
		if i < len(p.Bindings) {
			bindings = p.Bindings[i]
		} else {
			bindings = map[string]string{}
		}

		if err != nil {
			t.Diagnostic(
				fmt.Sprintf(`Query File: '%s'
Bindings File: '%s'
Bindings Name: '%s'
Query Parameters: '%v'
Expected Result File: '%s'
Actual Result File: '%s'

Failed to compare results: %s`,
					p.Query.Path,
					p.Path,
					bindingName,
					bindings,
					expectedFilename,
					rs.Filename,
					err.Error()))
		}

		if diff != "" {
			t.Diagnostic(
				fmt.Sprintf(`Query File: '%s'
Bindings File: '%s'
Bindings Name: '%s'
Query Parameters: '%v'
Expected Result File: '%s'
Actual Result File: '%s'

%s`,
					p.Query.Path,
					p.Path,
					bindingName,
					bindings,
					expectedFilename,
					rs.Filename,
					diff))
		}
		t.Ok(diff == "", testName)
	}
}

/*
CompareBaselines loads baseline files and compares actual query costs against them.
Uses EXPLAIN to get current costs and compares against stored baselines.
Threshold is the maximum allowed percentage increase (e.g., 10.0 for 10%).
*/
func (p *Plan) CompareBaselines(baselineDir string, db *sql.DB, t *tap.T, thresholdPercent float64) {
	// For queries without parameters, check single baseline
	if len(p.Query.Args) == 0 {
		p.compareSingleBaseline(baselineDir, "", db, t, thresholdPercent)
		return
	}

	// For queries with parameters, check baseline for each binding
	for i, bindings := range p.Bindings {
		bindingName := p.Names[i]
		p.compareBindingBaseline(baselineDir, bindingName, bindings, db, t, thresholdPercent)
	}
}

// compareSingleBaseline compares a single baseline for queries without parameters
func (p *Plan) compareSingleBaseline(baselineDir string, bindingName string, db *sql.DB, t *tap.T, thresholdPercent float64) {
	baselinePath := getBaselinePath(p.Query, baselineDir, bindingName)
	testName := strings.TrimSuffix(filepath.Base(baselinePath), ".yaml") + ".cost"

	// Load baseline
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		// If baseline doesn't exist, skip the test
		t.Skip(1, fmt.Sprintf("%s (no baseline)", testName))
		return
	}

	// Get current EXPLAIN cost
	explainPlan, err := ExecuteExplain(db, p.Query.OrdinalQuery)
	if err != nil {
		t.Diagnostic(fmt.Sprintf("Failed to execute EXPLAIN for %s: %s", testName, err.Error()))
		t.Ok(false, testName)
		return
	}

	// Extract current cost
	var actualCost float64
	if planData, ok := explainPlan["Plan"].(map[string]interface{}); ok {
		if cost, ok := planData["Total Cost"]; ok {
			actualCost = toFloat64(cost)
		}
	}

	// Extract baseline cost
	baselineCost := toFloat64(baseline.Plan["total_cost"])

	// Compare costs
	isOk, percentageIncrease := CompareCost(actualCost, baselineCost, thresholdPercent)

	// Format test name with actual values
	if isOk {
		detailedTestName := fmt.Sprintf("%s (%.2f <= %.2f * %.0f%%)", testName, actualCost, baselineCost, 100+thresholdPercent)
		t.Ok(true, detailedTestName)
	} else {
		detailedTestName := fmt.Sprintf("%s (%.2f > %.2f * %.0f%%, +%.1f%%)", testName, actualCost, baselineCost, 100+thresholdPercent, percentageIncrease)
		t.Diagnostic(fmt.Sprintf("Cost increased by %.1f%% (threshold: %.0f%%)", percentageIncrease, thresholdPercent))
		t.Ok(false, detailedTestName)
	}
}

// compareBindingBaseline compares a baseline for queries with specific parameter bindings
func (p *Plan) compareBindingBaseline(baselineDir string, bindingName string, bindings map[string]string, db *sql.DB, t *tap.T, thresholdPercent float64) {
	baselinePath := getBaselinePath(p.Query, baselineDir, bindingName)
	testName := strings.TrimSuffix(filepath.Base(baselinePath), ".yaml") + ".cost"

	// Load baseline
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		// If baseline doesn't exist, skip the test
		t.Skip(1, fmt.Sprintf("%s (no baseline)", testName))
		return
	}

	// Prepare query with bindings
	sql, args := p.Query.Prepare(bindings)

	// Get current EXPLAIN cost
	explainPlan, err := ExecuteExplain(db, sql, args...)
	if err != nil {
		t.Diagnostic(fmt.Sprintf("Failed to execute EXPLAIN for %s: %s", testName, err.Error()))
		t.Ok(false, testName)
		return
	}

	// Extract current cost
	var actualCost float64
	if planData, ok := explainPlan["Plan"].(map[string]interface{}); ok {
		if cost, ok := planData["Total Cost"]; ok {
			actualCost = toFloat64(cost)
		}
	}

	// Extract baseline cost
	baselineCost := toFloat64(baseline.Plan["total_cost"])

	// Compare costs
	isOk, percentageIncrease := CompareCost(actualCost, baselineCost, thresholdPercent)

	// Format test name with actual values
	if isOk {
		detailedTestName := fmt.Sprintf("%s (%.2f <= %.2f * %.0f%%)", testName, actualCost, baselineCost, 100+thresholdPercent)
		t.Ok(true, detailedTestName)
	} else {
		detailedTestName := fmt.Sprintf("%s (%.2f > %.2f * %.0f%%, +%.1f%%)", testName, actualCost, baselineCost, 100+thresholdPercent, percentageIncrease)
		t.Diagnostic(fmt.Sprintf("Cost increased by %.1f%% (threshold: %.0f%%)", percentageIncrease, thresholdPercent))
		t.Ok(false, detailedTestName)
	}
}

// toFloat64 converts an interface{} to float64
func toFloat64(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}
