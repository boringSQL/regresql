package regresql

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/pmezard/go-difflib/difflib"
)

type (
	ComparisonResult struct {
		TestName string
		Passed   bool
		Expected string
		Actual   string
		Diff     string
	}

	CostResult struct {
		TestName        string
		Passed          bool
		ActualCost      float64
		BaselineCost    float64
		PercentIncrease float64
		Error           string

		// Plan analysis
		PlanChanged     bool
		PlanRegressions []PlanRegression
		PlanWarnings    []PlanWarning
	}
)

func (p *Plan) CompareResultsData(expected []ResultSet) []ComparisonResult {
	results := make([]ComparisonResult, len(p.ResultSets))

	for i, actual := range p.ResultSets {
		if i >= len(expected) {
			actualJSON, _ := json.MarshalIndent(actual, "", "  ")
			results[i] = ComparisonResult{
				TestName: p.Names[i],
				Actual:   string(actualJSON),
				Diff:     "no expected result",
			}
			continue
		}

		actualJSON, err1 := json.MarshalIndent(actual, "", "  ")
		expectedJSON, err2 := json.MarshalIndent(expected[i], "", "  ")
		if err1 != nil || err2 != nil {
			results[i] = ComparisonResult{
				TestName: p.Names[i],
				Expected: string(expectedJSON),
				Actual:   string(actualJSON),
				Diff:     fmt.Sprintf("marshal error: %v, %v", err1, err2),
			}
			continue
		}

		actualStr, expectedStr := string(actualJSON), string(expectedJSON)
		if actualStr == expectedStr {
			results[i] = ComparisonResult{
				TestName: p.Names[i],
				Passed:   true,
				Expected: expectedStr,
				Actual:   actualStr,
			}
			continue
		}

		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(expectedStr),
			B:        difflib.SplitLines(actualStr),
			FromFile: "expected",
			ToFile:   "actual",
			Context:  3,
		}
		diffText, _ := difflib.GetUnifiedDiffString(diff)
		results[i] = ComparisonResult{
			TestName: p.Names[i],
			Expected: expectedStr,
			Actual:   actualStr,
			Diff:     diffText,
		}
	}

	return results
}

func (p *Plan) CompareCostsData(db *sql.DB, baselines []Baseline, thresholdPercent float64) []CostResult {
	results := make([]CostResult, len(p.Names))

	for i, name := range p.Names {
		if i >= len(baselines) {
			results[i] = CostResult{TestName: name, Error: "no baseline"}
			continue
		}

		var explainPlan map[string]any
		var err error
		if len(p.Query.Args) == 0 {
			explainPlan, err = ExecuteExplain(db, p.Query.OrdinalQuery)
		} else {
			sql, args := p.Query.Prepare(p.Bindings[i])
			explainPlan, err = ExecuteExplain(db, sql, args...)
		}
		if err != nil {
			results[i] = CostResult{TestName: name, Error: err.Error()}
			continue
		}

		actualCost := 0.0
		if planData, ok := explainPlan["Plan"].(map[string]any); ok {
			if cost, ok := planData["Total Cost"]; ok {
				actualCost = toFloat64(cost)
			}
		}

		baselineCost := toFloat64(baselines[i].Plan["total_cost"])
		passed, percentIncrease := CompareCost(actualCost, baselineCost, thresholdPercent)

		result := CostResult{
			TestName:        name,
			Passed:          passed,
			ActualCost:      actualCost,
			BaselineCost:    baselineCost,
			PercentIncrease: percentIncrease,
		}

		if baselines[i].PlanSignature != nil {
			currentSig, err := ExtractPlanSignature(explainPlan)
			if err == nil {
				result.PlanChanged = HasPlanChanged(baselines[i].PlanSignature, currentSig)
				result.PlanRegressions = DetectPlanRegressions(baselines[i].PlanSignature, currentSig)

				for _, regression := range result.PlanRegressions {
					if regression.Severity == "critical" {
						result.Passed = false
						break
					}
				}
			}
		}

		// Detect quality issues (works even without baseline)
		if explainPlan != nil {
			currentSig, err := ExtractPlanSignature(explainPlan)
			if err == nil {
				opts := p.Query.GetRegressQLOptions()
				ignoredTables := GetIgnoredSeqScanTables()
				result.PlanWarnings = DetectPlanQualityIssues(currentSig, opts, ignoredTables)
			}
		}

		results[i] = result
	}

	return results
}

func (p *Plan) CreateBaselines(db *sql.DB) ([]Baseline, []map[string]any, error) {
	baselines := make([]Baseline, len(p.Names))
	fullPlans := make([]map[string]any, len(p.Names))

	for i := range p.Names {
		baseline, fullPlan, err := p.createSingleBaseline(db, i)
		if err != nil {
			return nil, nil, err
		}
		baselines[i] = baseline
		fullPlans[i] = fullPlan
	}

	return baselines, fullPlans, nil
}

func (p *Plan) createSingleBaseline(db *sql.DB, index int) (Baseline, map[string]any, error) {
	var explainPlan map[string]any
	var err error

	if len(p.Query.Args) == 0 {
		explainPlan, err = ExecuteExplain(db, p.Query.OrdinalQuery)
	} else {
		sql, args := p.Query.Prepare(p.Bindings[index])
		explainPlan, err = ExecuteExplain(db, sql, args...)
	}
	if err != nil {
		return Baseline{}, nil, fmt.Errorf("failed to create baseline for %s: %w", p.Names[index], err)
	}

	filteredPlan := make(map[string]any)
	if planData, ok := explainPlan["Plan"].(map[string]any); ok {
		if startupCost, ok := planData["Startup Cost"]; ok {
			filteredPlan["startup_cost"] = startupCost
		}
		if totalCost, ok := planData["Total Cost"]; ok {
			filteredPlan["total_cost"] = totalCost
		}
		if planRows, ok := planData["Plan Rows"]; ok {
			filteredPlan["plan_rows"] = planRows
		}
	}

	return Baseline{Query: p.Query.Name, Plan: filteredPlan}, explainPlan, nil
}
