package regresql

import (
	"encoding/json"
	"fmt"
	"os"
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

func (p *Plan) CompareBaselines(baselineDir string, q Querier, t *tap.T, thresholdPercent float64) {
	for _, r := range p.CompareBaselinesToResults(baselineDir, q, thresholdPercent) {
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

func toFloat64(val any) float64 {
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

func loadResultSet(filename string) (*ResultSet, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var rs ResultSet
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, err
	}

	return &rs, nil
}

func parseFloatOption(value string) float64 {
	var f float64
	fmt.Sscanf(value, "%f", &f)
	return f
}

func (p *Plan) CompareResultSetsToResults(regressDir, expectedDir string) []TestResult {
	results := make([]TestResult, 0, len(p.ResultSets))
	diffConfig := GetDiffConfig()

	for i, actualRS := range p.ResultSets {
		start := time.Now()
		testName := strings.TrimPrefix(actualRS.Filename, regressDir+"/out/")
		expectedFilename := filepath.Join(expectedDir, filepath.Base(actualRS.Filename))

		bindingName := "n/a"
		if i < len(p.Names) {
			bindingName = p.Names[i]
		}

		bindings := map[string]any{}
		if i < len(p.Bindings) {
			bindings = p.Bindings[i]
		}

		// Apply per-query diff options if available
		queryDiffConfig := diffConfig
		if p.Query != nil {
			opts := p.Query.GetRegressQLOptions()
			if opts.DiffFloatTolerance > 0 {
				queryDiffConfig = &DiffConfig{
					FloatTolerance: opts.DiffFloatTolerance,
					MaxSamples:     diffConfig.MaxSamples,
				}
			}
		}

		result := TestResult{
			Name:         testName,
			Type:         "output",
			Duration:     0, // will be set at the end
			QueryFile:    p.Query.Path,
			BindingsFile: p.Path,
			BindingName:  bindingName,
			Parameters:   bindings,
		}

		// Try to load expected result set and perform semantic comparison
		expectedRS, err := loadResultSet(expectedFilename)
		if err != nil {
			// Fall back to text diff if we can't load expected as JSON
			diff, diffErr := DiffFiles(expectedFilename, actualRS.Filename, 3)
			if diffErr != nil {
				result.Status = "failed"
				result.Error = fmt.Sprintf("Failed to compare results: %s", diffErr.Error())
			} else if diff != "" {
				result.Status = "failed"
				result.Diff = diff
			} else {
				result.Status = "passed"
			}
		} else {
			// Perform semantic comparison
			structuredDiff := CompareResultSets(expectedRS, &actualRS, queryDiffConfig)
			result.StructuredDiff = structuredDiff

			if !structuredDiff.Identical {
				result.Status = "failed"
				// Also generate text diff for backward compatibility
				textDiff, _ := DiffFiles(expectedFilename, actualRS.Filename, 3)
				result.Diff = textDiff
			} else {
				result.Status = "passed"
			}
		}

		result.Duration = time.Since(start).Seconds()
		results = append(results, result)
	}

	return results
}

func (p *Plan) CompareBaselinesToResults(baselineDir string, q Querier, thresholdPercent float64) []TestResult {
	if len(p.Query.Args) == 0 {
		return []TestResult{p.compareBaseline(baselineDir, "", nil, q, thresholdPercent)}
	}

	results := make([]TestResult, 0, len(p.Bindings))
	for i, bindings := range p.Bindings {
		result := p.compareBaseline(baselineDir, p.Names[i], bindings, q, thresholdPercent)
		results = append(results, result)
	}
	return results
}

func (p *Plan) compareBaseline(baselineDir, bindingName string, bindings map[string]any, q Querier, thresholdPercent float64) TestResult {
	start := time.Now()
	baselinePath := getBaselinePath(p.Query, baselineDir, bindingName)
	testName := strings.TrimSuffix(filepath.Base(baselinePath), ".json") + ".cost"

	result := TestResult{
		Name:       testName,
		Type:       "cost",
		Threshold:  thresholdPercent,
		Parameters: bindings,
	}

	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		result.Status = "skipped"
		result.Error = "no baseline"
		result.Duration = time.Since(start).Seconds()
		return result
	}

	explainPlan, err := p.runExplain(q, bindings)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("Failed to execute EXPLAIN: %s", err.Error())
		result.Duration = time.Since(start).Seconds()
		return result
	}

	actualCost := explainPlan.Plan.TotalCost
	baselineCost := toFloat64(baseline.Plan["total_cost"])
	isOk, percentageIncrease := CompareCost(actualCost, baselineCost, thresholdPercent)

	result.ActualCost = actualCost
	result.ExpectedCost = baselineCost
	result.PercentIncrease = percentageIncrease
	result.Duration = time.Since(start).Seconds()

	currentSig := ExtractPlanSignatureFromNode(&explainPlan.Plan)

	if baseline.PlanSignature != nil {
		result.PlanChanged = HasPlanChanged(baseline.PlanSignature, currentSig)
		result.PlanRegressions = DetectPlanRegressions(baseline.PlanSignature, currentSig)

		if hasCriticalRegression(result.PlanRegressions) {
			isOk = false
		}
	}

	opts := p.Query.GetRegressQLOptions()
	result.PlanWarnings = DetectPlanQualityIssues(currentSig, opts, GetIgnoredSeqScanTables())

	if isOk {
		result.Status = "passed"
		result.Name = fmt.Sprintf("%s (%.2f <= %.2f * %.0f%%)", testName, actualCost, baselineCost, 100+thresholdPercent)
	} else {
		result.Status = "failed"
		result.Name = fmt.Sprintf("%s (%.2f > %.2f * %.0f%%, +%.1f%%)", testName, actualCost, baselineCost, 100+thresholdPercent, percentageIncrease)
	}

	return result
}

func (p *Plan) runExplain(q Querier, bindings map[string]any) (*ExplainOutput, error) {
	if bindings == nil {
		return ExecuteExplain(q, p.Query.OrdinalQuery)
	}
	sql, args := p.Query.Prepare(bindings)
	return ExecuteExplain(q, sql, args...)
}

func hasCriticalRegression(regressions []PlanRegression) bool {
	for _, reg := range regressions {
		if reg.Severity == "critical" {
			return true
		}
	}
	return false
}
