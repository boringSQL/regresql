package regresql

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mndrix/tap-go"
)

func (p *Plan) CompareResultSets(regressDir, expectedDir string, t *tap.T) {
	for _, r := range p.CompareResultSetsToResults(regressDir, expectedDir) {
		outputResultToTAP(r, t)
	}
}

func (p *Plan) CompareBaselines(baselineDir string, db *sql.DB, t *tap.T, thresholdPercent float64) {
	for _, r := range p.CompareBaselinesToResults(baselineDir, db, thresholdPercent) {
		outputResultToTAP(r, t)
	}
}

func outputResultToTAP(r TestResult, t *tap.T) {
	if r.Status == "failed" {
		if r.Type == "output" {
			t.Diagnostic(fmt.Sprintf(`Query File: '%s'
Bindings File: '%s'
Bindings Name: '%s'
Query Parameters: '%v'

%s`, r.QueryFile, r.BindingsFile, r.BindingName, r.Parameters, r.Diff))
		} else if r.Type == "cost" {
			t.Diagnostic(fmt.Sprintf("Cost increased by %.1f%% (threshold: %.0f%%)",
				r.PercentIncrease, r.Threshold))
		}
	}

	if r.Error != "" {
		t.Diagnostic(r.Error)
	}

	switch r.Status {
	case "passed":
		t.Ok(true, r.Name)
	case "failed":
		t.Ok(false, r.Name)
	case "skipped":
		t.Skip(1, r.Name)
	}
}

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

/*
CompareResultSetsToResults compares result sets and returns TestResult structs
instead of writing to TAP output. This is used by the formatter system.
*/
func (p *Plan) CompareResultSetsToResults(regressDir string, expectedDir string) []TestResult {
	results := make([]TestResult, 0, len(p.ResultSets))

	for i, rs := range p.ResultSets {
		start := time.Now()
		testName := strings.TrimPrefix(rs.Filename, regressDir+"/out/")
		expectedFilename := filepath.Join(expectedDir, filepath.Base(rs.Filename))

		// Get binding info if available
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

		diff, err := DiffFiles(expectedFilename, rs.Filename, 3)
		duration := time.Since(start).Seconds()

		result := TestResult{
			Name:         testName,
			Type:         "output",
			Duration:     duration,
			QueryFile:    p.Query.Path,
			BindingsFile: p.Path,
			BindingName:  bindingName,
			Parameters:   bindings,
		}

		if err != nil {
			result.Status = "failed"
			result.Error = fmt.Sprintf("Failed to compare results: %s", err.Error())
		} else if diff != "" {
			result.Status = "failed"
			result.Diff = diff
		} else {
			result.Status = "passed"
		}

		results = append(results, result)
	}

	return results
}

/*
CompareBaselinesToResults compares baselines and returns TestResult structs
instead of writing to TAP output. This is used by the formatter system.
*/
func (p *Plan) CompareBaselinesToResults(baselineDir string, db *sql.DB, thresholdPercent float64) []TestResult {
	results := make([]TestResult, 0)

	// For queries without parameters, check single baseline
	if len(p.Query.Args) == 0 {
		result := p.compareSingleBaselineToResult(baselineDir, "", db, thresholdPercent)
		results = append(results, result)
		return results
	}

	// For queries with parameters, check baseline for each binding
	for i, bindings := range p.Bindings {
		bindingName := p.Names[i]
		result := p.compareBindingBaselineToResult(baselineDir, bindingName, bindings, db, thresholdPercent)
		results = append(results, result)
	}

	return results
}

// compareSingleBaselineToResult compares a single baseline and returns a TestResult
func (p *Plan) compareSingleBaselineToResult(baselineDir string, bindingName string, db *sql.DB, thresholdPercent float64) TestResult {
	start := time.Now()
	baselinePath := getBaselinePath(p.Query, baselineDir, bindingName)
	testName := strings.TrimSuffix(filepath.Base(baselinePath), ".json") + ".cost"

	result := TestResult{
		Name:      testName,
		Type:      "cost",
		Threshold: thresholdPercent,
	}

	// Load baseline
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		result.Status = "skipped"
		result.Error = "no baseline"
		result.Duration = time.Since(start).Seconds()
		return result
	}

	// Get current EXPLAIN cost
	explainPlan, err := ExecuteExplain(db, p.Query.OrdinalQuery)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("Failed to execute EXPLAIN: %s", err.Error())
		result.Duration = time.Since(start).Seconds()
		return result
	}

	// Extract costs
	var actualCost float64
	if planData, ok := explainPlan["Plan"].(map[string]interface{}); ok {
		if cost, ok := planData["Total Cost"]; ok {
			actualCost = toFloat64(cost)
		}
	}
	baselineCost := toFloat64(baseline.Plan["total_cost"])

	// Compare costs
	isOk, percentageIncrease := CompareCost(actualCost, baselineCost, thresholdPercent)

	result.ActualCost = actualCost
	result.ExpectedCost = baselineCost
	result.PercentIncrease = percentageIncrease
	result.Duration = time.Since(start).Seconds()

	if isOk {
		result.Status = "passed"
		result.Name = fmt.Sprintf("%s (%.2f <= %.2f * %.0f%%)", testName, actualCost, baselineCost, 100+thresholdPercent)
	} else {
		result.Status = "failed"
		result.Name = fmt.Sprintf("%s (%.2f > %.2f * %.0f%%, +%.1f%%)", testName, actualCost, baselineCost, 100+thresholdPercent, percentageIncrease)
	}

	return result
}

// compareBindingBaselineToResult compares a baseline with bindings and returns a TestResult
func (p *Plan) compareBindingBaselineToResult(baselineDir string, bindingName string, bindings map[string]string, db *sql.DB, thresholdPercent float64) TestResult {
	start := time.Now()
	baselinePath := getBaselinePath(p.Query, baselineDir, bindingName)
	testName := strings.TrimSuffix(filepath.Base(baselinePath), ".json") + ".cost"

	result := TestResult{
		Name:       testName,
		Type:       "cost",
		Threshold:  thresholdPercent,
		Parameters: bindings,
	}

	// Load baseline
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		result.Status = "skipped"
		result.Error = "no baseline"
		result.Duration = time.Since(start).Seconds()
		return result
	}

	// Prepare query with bindings
	sql, args := p.Query.Prepare(bindings)

	// Get current EXPLAIN cost
	explainPlan, err := ExecuteExplain(db, sql, args...)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("Failed to execute EXPLAIN: %s", err.Error())
		result.Duration = time.Since(start).Seconds()
		return result
	}

	// Extract costs
	var actualCost float64
	if planData, ok := explainPlan["Plan"].(map[string]interface{}); ok {
		if cost, ok := planData["Total Cost"]; ok {
			actualCost = toFloat64(cost)
		}
	}
	baselineCost := toFloat64(baseline.Plan["total_cost"])

	// Compare costs
	isOk, percentageIncrease := CompareCost(actualCost, baselineCost, thresholdPercent)

	result.ActualCost = actualCost
	result.ExpectedCost = baselineCost
	result.PercentIncrease = percentageIncrease
	result.Duration = time.Since(start).Seconds()

	if isOk {
		result.Status = "passed"
		result.Name = fmt.Sprintf("%s (%.2f <= %.2f * %.0f%%)", testName, actualCost, baselineCost, 100+thresholdPercent)
	} else {
		result.Status = "failed"
		result.Name = fmt.Sprintf("%s (%.2f > %.2f * %.0f%%, +%.1f%%)", testName, actualCost, baselineCost, 100+thresholdPercent, percentageIncrease)
	}

	return result
}
