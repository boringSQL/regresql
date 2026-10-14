package regresql

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanNodeSchema(t *testing.T) {
	const verbose = `[{"Plan":{"Node Type":"Index Scan","Index Name":"s_t_pkey","Relation Name":"s_t","Schema":"public","Alias":"s_t","Startup Cost":0.15,"Total Cost":8.17,"Plan Rows":1,"Plan Width":8,"Output":["a","b"]}}]`
	const plain = `[{"Plan":{"Node Type":"Index Scan","Relation Name":"s_t","Alias":"s_t","Startup Cost":0.15,"Total Cost":8.17,"Plan Rows":1,"Plan Width":8}}]`

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"verbose", verbose, "public"},
		{"plain", plain, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out []ExplainOutput
			if err := json.Unmarshal([]byte(tt.in), &out); err != nil {
				t.Fatal(err)
			}
			if got := out[0].Plan.Schema; got != tt.want {
				t.Errorf("Schema = %q, want %q", got, tt.want)
			}

			if tt.want == "" {
				b, err := json.Marshal(out[0].Plan)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(b), `"Schema"`) {
					t.Errorf("empty Schema not omitted: %s", b)
				}
			}
		})
	}
}
