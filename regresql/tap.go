package regresql

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

		// Check if expected file exists - mark as pending if missing
		if _, err := os.Stat(expectedFilename); os.IsNotExist(err) {
			result.Status = "pending"
			result.Error = fmt.Sprintf("Expected file not found: %s (run 'regresql update' to create)", expectedFilename)
			result.Duration = time.Since(start).Seconds()
			results = append(results, result)
			continue
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

func (p *Plan) CompareBaselinesToResults(ctx context.Context, baselineDir string, q Querier, thresholdPercent float64) []TestResult {
	if len(p.Query.Args) == 0 {
		return []TestResult{p.compareBaseline(ctx, baselineDir, "", nil, q, thresholdPercent)}
	}

	results := make([]TestResult, 0, len(p.Bindings))
	for i, bindings := range p.Bindings {
		result := p.compareBaseline(ctx, baselineDir, p.Names[i], bindings, q, thresholdPercent)
		results = append(results, result)
	}
	return results
}

func (p *Plan) compareBaseline(ctx context.Context, baselineDir, bindingName string, bindings map[string]any, q Querier, thresholdPercent float64) TestResult {
	start := time.Now()
	baselinePath := getBaselinePath(p.Query, baselineDir, bindingName)
	testName := strings.TrimSuffix(filepath.Base(baselinePath), ".json") + ".cost"

	planChoice := CurrentMode() == ModePlanChoice

	result := TestResult{
		Name:       testName,
		Type:       "cost",
		Threshold:  thresholdPercent,
		Parameters: bindings,
		PlanChoice: planChoice,
	}

	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		result.Status = "skipped"
		result.Error = "no baseline"
		result.Duration = time.Since(start).Seconds()
		return result
	}

	// Warn if analyze.enabled but baseline lacks buffer data
	if !planChoice && IsAnalyzeEnabled() && !baseline.AnalyzeMode {
		fmt.Fprintf(os.Stderr, "Warning: analyze.enabled=true but baseline '%s' lacks buffer data\n", baselinePath)
	}

	var useBufferComparison bool
	if !planChoice {
		useBufferComparison, err = getComparisonMode(baseline, baselinePath)
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			result.Duration = time.Since(start).Seconds()
			return result
		}
	}

	explainPlan, err := p.runExplainWithMode(ctx, q, bindings, useBufferComparison)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("Failed to execute EXPLAIN: %s", err.Error())
		result.Duration = time.Since(start).Seconds()
		return result
	}

	var isOk, bufferOk bool
	var percentageIncrease float64
	improvementThreshold := GetImprovementThreshold()

	if useBufferComparison {
		actualBuffers := explainPlan.Plan.SharedHitBlocks + explainPlan.Plan.SharedReadBlocks + explainPlan.Plan.LocalHitBlocks + explainPlan.Plan.LocalReadBlocks
		baselineBuffers := baseline.Buffers.TotalBuffers
		bufferThreshold := GetBufferThreshold()

		isOk, percentageIncrease = CompareBuffers(actualBuffers, baselineBuffers, bufferThreshold)
		bufferOk = isOk

		// spill fails the check even when shared buffers pass
		tr, tw := SumTempBlocks(&explainPlan.Plan)
		actualTemp := tr + tw
		baselineTemp := baseline.Buffers.TempBuffers
		if IsSpillRegression(actualTemp, baselineTemp, bufferThreshold) {
			result.SpillRegression = true
			isOk = false
		}

		// tuple-flow: advisory only (plan-shape changes legitimately move more rows)
		actualTuples := SumTuplesProcessed(&explainPlan.Plan)
		var baselineTuples float64
		if baseline.Actuals != nil {
			baselineTuples = baseline.Actuals.TotalTuplesProcessed
		}
		tupleOk, tupleIncrease := CompareTuples(actualTuples, baselineTuples, bufferThreshold)
		result.ActualTuples = actualTuples
		result.BaselineTuples = baselineTuples
		result.TupleIncrease = tupleIncrease
		result.TupleRegression = !tupleOk

		// q-error gate (estimation quality)
		if worst := WorstQError(&explainPlan.Plan); worst != nil {
			result.ActualQError = worst.QError
			result.QErrorNode = qErrorNodeLabel(worst)
		}
		if baseline.Actuals != nil {
			result.BaselineQError = baseline.Actuals.WorstQError
		}
		if IsQErrorRegression(result.ActualQError, result.BaselineQError, GetQErrorRatio(), GetQErrorFloor()) {
			result.QErrorRegression = true
			isOk = false
		}

		result.AnalyzeMode = true
		result.ActualBuffers = actualBuffers
		result.BaselineBuffers = baselineBuffers
		result.BufferIncrease = percentageIncrease
		result.ActualTempBuffers = actualTemp
		result.BaselineTempBuffers = baselineTemp
		result.Threshold = bufferThreshold
		result.ActualCost = explainPlan.Plan.TotalCost
		result.ExpectedCost = toFloat64(baseline.Plan["total_cost"])
		result.PercentIncrease = percentageIncrease

		// Detect significant improvement
		if baselineBuffers > 0 {
			improvementPercent := (float64(baselineBuffers) - float64(actualBuffers)) / float64(baselineBuffers) * 100
			if improvementPercent >= improvementThreshold {
				result.Improved = true
				result.ImprovementPercent = improvementPercent
			}
		}

		testName = strings.TrimSuffix(filepath.Base(baselinePath), ".json") + ".buffers"
	} else {
		actualCost := explainPlan.Plan.TotalCost
		baselineCost := toFloat64(baseline.Plan["total_cost"])
		costThreshold := GetCostThreshold()

		isOk, percentageIncrease = CompareCost(actualCost, baselineCost, costThreshold)

		result.ActualCost = actualCost
		result.ExpectedCost = baselineCost
		result.PercentIncrease = percentageIncrease
		result.Threshold = costThreshold

		// Detect significant improvement
		if baselineCost > 0 {
			improvementPercent := (baselineCost - actualCost) / baselineCost * 100
			if improvementPercent >= improvementThreshold {
				result.Improved = true
				result.ImprovementPercent = improvementPercent
			}
		}
	}

	result.Duration = time.Since(start).Seconds()

	currentSig := ExtractPlanSignatureFromNode(&explainPlan.Plan)

	if baseline.PlanSignature != nil {
		result.PlanChanged = HasPlanChanged(baseline.PlanSignature, currentSig)
		result.PlanRegressions = DetectPlanRegressions(baseline.PlanSignature, currentSig)
	}

	isOk, notes := gateVerdict(verdictInput{
		Mode:         CurrentMode(),
		BranchOk:     isOk,
		PlanChanged:  result.PlanChanged,
		HasSignature: baseline.PlanSignature != nil,
		Regressions:  result.PlanRegressions,
	})

	opts := p.Query.GetRegressQLOptions()
	costInfo := planCostInfo(&explainPlan.Plan, planChoice && !useBufferComparison)
	result.PlanWarnings = DetectPlanQualityIssues(currentSig, opts, GetIgnoredSeqScanTables(), GetCriticalTables(), costInfo)

	if planChoice {
		result.Improved = false
		result.ImprovementPercent = 0
		result.PlanWarnings = append(result.PlanWarnings, notes...)
		if percentageIncrease > result.Threshold {
			result.PlanWarnings = append(result.PlanWarnings, PlanWarning{
				Type:     CostIncreased,
				Severity: "info",
				Message:  fmt.Sprintf("Cost %.2f vs baseline %.2f (+%.1f%%), plan shape unchanged", result.ActualCost, result.ExpectedCost, percentageIncrease),
			})
		}
		result.BaselineUninjected = !baseline.StatsInjected
		result.Status, result.Name = planChoiceResult(testName, isOk, &result)
		return result
	}

	if useBufferComparison {
		result.Status, result.Name = bufferResult(testName, isOk, bufferOk, &result, percentageIncrease)
	} else {
		result.Status, result.Name = legacyCostResult(testName, isOk, &result, percentageIncrease)
	}

	return result
}

func (p *Plan) runExplainWithMode(ctx context.Context, q Querier, bindings map[string]any, useAnalyze bool) (*ExplainOutput, error) {
	opts := DefaultExplainOptions()
	if useAnalyze {
		opts.Analyze = true
		opts.Buffers = true
	}

	if bindings == nil {
		return ExecuteExplainWithOptions(ctx, q, p.Query.OrdinalQuery, opts)
	}
	sql, args := p.Query.Prepare(bindings)
	return ExecuteExplainWithOptions(ctx, q, sql, opts, args...)
}

type (
	verdictInput struct {
		Mode         Mode
		BranchOk     bool
		PlanChanged  bool
		HasSignature bool
		Regressions  []PlanRegression
	}
)

// gateVerdict decides pass/fail. Buffers mode is the legacy rule; plan choice
// mode gates on plan shape only and never on cost.
func gateVerdict(in verdictInput) (bool, []PlanWarning) {
	critical := hasCriticalRegression(in.Regressions)
	if in.Mode != ModePlanChoice {
		return in.BranchOk && !critical, nil
	}
	if !in.HasSignature {
		return true, []PlanWarning{{
			Type:       NoPlanSignature,
			Severity:   "info",
			Message:    "baseline has no plan signature; plan shape was not compared",
			Suggestion: "re-run regresql baseline",
		}}
	}
	return !in.PlanChanged && !critical, nil
}

func planCostInfo(n *PlanNode, buffersUnavailable bool) PlanCostInfo {
	info := PlanCostInfo{
		TotalCost:    n.TotalCost,
		TotalBuffers: n.SharedHitBlocks + n.SharedReadBlocks + n.LocalHitBlocks + n.LocalReadBlocks,
	}
	if buffersUnavailable {
		info.TotalBuffers = -1
	}
	return info
}

func bufferResult(testName string, isOk, bufferOk bool, r *TestResult, percentageIncrease float64) (string, string) {
	var status, name string
	qErrorNamed := false
	switch {
	case r.SpillRegression:
		status = "failed"
		name = fmt.Sprintf("%s (spill: temp %d > %d blocks)", testName, r.ActualTempBuffers, r.BaselineTempBuffers)
	case isOk:
		status = "passed"
		name = fmt.Sprintf("%s (%d <= %d * %.0f%%)", testName, r.ActualBuffers, r.BaselineBuffers, 100+r.Threshold)
	case bufferOk && r.QErrorRegression:
		qErrorNamed = true
		status = "failed"
		name = fmt.Sprintf("%s (q-error %.0fx > baseline %.0fx, %s)", testName, r.ActualQError, r.BaselineQError, r.QErrorNode)
	default:
		status = "failed"
		name = fmt.Sprintf("%s (%d > %d * %.0f%%, +%.1f%%)", testName, r.ActualBuffers, r.BaselineBuffers, 100+r.Threshold, percentageIncrease)
	}

	if r.TupleRegression {
		name += fmt.Sprintf(" [tuples +%.1f%%]", r.TupleIncrease)
	}
	if r.QErrorRegression && !qErrorNamed {
		name += fmt.Sprintf(" [q-error %.0fx > %.0fx]", r.ActualQError, r.BaselineQError)
	}
	return status, name
}

func legacyCostResult(testName string, ok bool, r *TestResult, percentageIncrease float64) (string, string) {
	if ok {
		return "passed", fmt.Sprintf("%s (%.2f <= %.2f * %.0f%%)", testName, r.ActualCost, r.ExpectedCost, 100+r.Threshold)
	}
	return "failed", fmt.Sprintf("%s (%.2f > %.2f * %.0f%%, +%.1f%%)", testName, r.ActualCost, r.ExpectedCost, 100+r.Threshold, percentageIncrease)
}

func planChoiceResult(testName string, ok bool, r *TestResult) (string, string) {
	if ok {
		return "passed", fmt.Sprintf("%s (shape unchanged; cost %.2f vs %.2f, advisory)", testName, r.ActualCost, r.ExpectedCost)
	}
	return "failed", fmt.Sprintf("%s (plan changed)", testName)
}

func hasCriticalRegression(regressions []PlanRegression) bool {
	for _, reg := range regressions {
		if reg.Severity == "critical" {
			return true
		}
	}
	return false
}

func getComparisonMode(baseline *Baseline, baselinePath string) (bool, error) {
	mode := GetComparisonMode()
	hasBuffers := baseline.AnalyzeMode && baseline.Buffers != nil

	switch mode {
	case "buffers":
		if !hasBuffers {
			return false, fmt.Errorf("comparison: buffers requires baseline with buffer data - regenerate '%s' with --analyze", baselinePath)
		}
		return true, nil
	case "cost":
		return false, nil
	default: // "auto"
		return hasBuffers, nil
	}
}
