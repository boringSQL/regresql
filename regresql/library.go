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

		Metrics *PlanMetrics // from EXPLAIN ANALYZE

		AnalyzeMode     bool
		ActualBuffers   int64
		BaselineBuffers int64
		BufferIncrease  float64
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

		var explainPlan *ExplainOutput
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

		actualCost := explainPlan.Plan.TotalCost
		baselineCost := toFloat64(baselines[i].Plan["total_cost"])
		passed, percentIncrease := CompareCost(actualCost, baselineCost, thresholdPercent)

		result := CostResult{
			TestName:        name,
			Passed:          passed,
			ActualCost:      actualCost,
			BaselineCost:    baselineCost,
			PercentIncrease: percentIncrease,
		}

		// Only populate metrics when ANALYZE was used
		if explainPlan.ExecutionTime > 0 {
			metrics := explainPlan.ExtractMetrics()
			result.Metrics = &metrics
		}

		if baselines[i].PlanSignature != nil {
			currentSig := ExtractPlanSignatureFromNode(&explainPlan.Plan)
			result.PlanChanged = HasPlanChanged(baselines[i].PlanSignature, currentSig)
			result.PlanRegressions = DetectPlanRegressions(baselines[i].PlanSignature, currentSig)

			for _, regression := range result.PlanRegressions {
				if regression.Severity == "critical" {
					result.Passed = false
					break
				}
			}
		}

		// Detect quality issues (works even without baseline)
		currentSig := ExtractPlanSignatureFromNode(&explainPlan.Plan)
		opts := p.Query.GetRegressQLOptions()
		ignoredTables := GetIgnoredSeqScanTables()
		result.PlanWarnings = DetectPlanQualityIssues(currentSig, opts, ignoredTables)

		results[i] = result
	}

	return results
}

func (p *Plan) CreateBaselines(db *sql.DB, useAnalyze bool) ([]Baseline, []*ExplainOutput, error) {
	baselines := make([]Baseline, len(p.Names))
	fullPlans := make([]*ExplainOutput, len(p.Names))

	for i := range p.Names {
		baseline, fullPlan, err := p.createSingleBaseline(db, i, useAnalyze)
		if err != nil {
			return nil, nil, err
		}
		baselines[i] = baseline
		fullPlans[i] = fullPlan
	}

	return baselines, fullPlans, nil
}

func (p *Plan) createSingleBaseline(db *sql.DB, index int, useAnalyze bool) (Baseline, *ExplainOutput, error) {
	var explainPlan *ExplainOutput
	var err error

	opts := DefaultExplainOptions()
	if useAnalyze {
		opts.Analyze = true
		opts.Buffers = true
	}

	if len(p.Query.Args) == 0 {
		explainPlan, err = ExecuteExplainWithOptions(db, p.Query.OrdinalQuery, opts)
	} else {
		sql, args := p.Query.Prepare(p.Bindings[index])
		explainPlan, err = ExecuteExplainWithOptions(db, sql, opts, args...)
	}
	if err != nil {
		return Baseline{}, nil, fmt.Errorf("failed to create baseline for %s: %w", p.Names[index], err)
	}

	filteredPlan := map[string]any{
		"startup_cost": explainPlan.Plan.StartupCost,
		"total_cost":   explainPlan.Plan.TotalCost,
		"plan_rows":    explainPlan.Plan.PlanRows,
	}

	return Baseline{Query: p.Query.Name, Plan: filteredPlan}, explainPlan, nil
}
