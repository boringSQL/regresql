package regresql

import (
	"encoding/json"
	"io"
	"time"
)

type JSONFormatter struct {
	results []TestResult
}

func (f *JSONFormatter) Start(w io.Writer) error {
	f.results = make([]TestResult, 0)
	return nil
}

func (f *JSONFormatter) AddResult(r TestResult, w io.Writer) error {
	f.results = append(f.results, r)
	return nil
}

func (f *JSONFormatter) Finish(s *TestSummary, w io.Writer) error {
	output := map[string]any{
		"version":   "1.0",
		"timestamp": s.StartTime.Format(time.RFC3339),
		"summary": map[string]any{
			"total":    s.Total,
			"passed":   s.Passed,
			"failed":   s.Failed,
			"skipped":  s.Skipped,
			"duration": s.Duration,
		},
		"tests": formatTests(f.results),
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func formatTests(results []TestResult) []map[string]any {
	tests := make([]map[string]any, 0, len(results))

	for _, r := range results {
		test := map[string]any{
			"name":     r.Name,
			"type":     r.Type,
			"status":   r.Status,
			"duration": r.Duration,
		}

		if r.Error != "" {
			test["error"] = r.Error
		}

		if r.Type == "cost" {
			if r.ExpectedCost > 0 {
				test["expected"] = map[string]any{
					"total_cost": r.ExpectedCost,
				}
			}
			if r.ActualCost > 0 {
				test["actual"] = map[string]any{
					"total_cost": r.ActualCost,
				}
			}
			if r.PercentIncrease != 0 {
				test["percent_increase"] = r.PercentIncrease
			}

			// Add plan analysis information
			if r.PlanChanged {
				test["plan_changed"] = true
			}

			if len(r.PlanRegressions) > 0 {
				regressions := make([]map[string]any, 0, len(r.PlanRegressions))
				for _, reg := range r.PlanRegressions {
					regression := map[string]any{
						"type":     string(reg.Type),
						"severity": reg.Severity,
						"message":  reg.Message,
					}
					if reg.Table != "" {
						regression["table"] = reg.Table
					}
					if reg.OldScan != "" {
						regression["old_scan"] = reg.OldScan
					}
					if reg.NewScan != "" {
						regression["new_scan"] = reg.NewScan
					}
					if reg.IndexName != "" {
						regression["index_name"] = reg.IndexName
					}
					if len(reg.Recommendations) > 0 {
						regression["recommendations"] = reg.Recommendations
					}
					regressions = append(regressions, regression)
				}
				test["plan_regressions"] = regressions
			}

			if len(r.PlanWarnings) > 0 {
				warnings := make([]map[string]any, 0, len(r.PlanWarnings))
				for _, warn := range r.PlanWarnings {
					warning := map[string]any{
						"type":     string(warn.Type),
						"severity": warn.Severity,
						"message":  warn.Message,
					}
					if warn.Table != "" {
						warning["table"] = warn.Table
					}
					if warn.Suggestion != "" {
						warning["suggestion"] = warn.Suggestion
					}
					warnings = append(warnings, warning)
				}
				test["plan_warnings"] = warnings
			}
		}

		if r.Type == "output" && r.Diff != "" {
			test["diff"] = r.Diff
		}

		tests = append(tests, test)
	}

	return tests
}

func init() {
	RegisterFormatter("json", &JSONFormatter{})
}
