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

		if r.Type == "cost" && r.Status == "failed" {
			test["expected"] = map[string]any{
				"total_cost": r.ExpectedCost,
			}
			test["actual"] = map[string]any{
				"total_cost": r.ActualCost,
			}
			if r.PercentIncrease > 0 {
				test["diff"] = map[string]any{
					"summary":          "Plan changed from Index Scan to Seq Scan",
					"percent_increase": r.PercentIncrease,
				}
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
