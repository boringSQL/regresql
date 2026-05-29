package regresql

import (
	"encoding/json"
	"testing"
)

// Temp Read/Written blocks from EXPLAIN BUFFERS must reach the metrics extractor.
func TestExtractMetrics_CapturesTempBlocks(t *testing.T) {
	raw := `{"Plan":{"Node Type":"Sort","Total Cost":100,"Temp Read Blocks":40,"Temp Written Blocks":48}}`

	var out ExplainOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	m := out.ExtractMetrics()
	if m.TempReadBlocks != 40 || m.TempWrittenBlocks != 48 {
		t.Errorf("temp blocks = %d/%d, want 40/48", m.TempReadBlocks, m.TempWrittenBlocks)
	}
}

func TestIsSpillRegression(t *testing.T) {
	cases := []struct {
		name             string
		actual, baseline int64
		want             bool
	}{
		{"no spill either side", 0, 0, false},
		{"started spilling", 100, 0, true},
		{"spill resolved", 0, 100, false},
		{"spill grew past threshold", 130, 100, true},
		{"spill within threshold", 105, 100, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsSpillRegression(tc.actual, tc.baseline, DefaultCostThresholdPercent)
			if got != tc.want {
				t.Errorf("IsSpillRegression(%d, %d) = %v, want %v", tc.actual, tc.baseline, got, tc.want)
			}
		})
	}
}
