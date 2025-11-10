package regresql

import (
	"fmt"
	"math/rand"
)

type RangeGenerator struct{}

func NewRangeGenerator() *RangeGenerator                  { return &RangeGenerator{} }
func (g *RangeGenerator) Name() string                    { return "range" }

func (g *RangeGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	valSlice, ok := params["values"].([]any)
	if !ok {
		return fmt.Errorf("range generator requires 'values' array")
	}
	if len(valSlice) == 0 {
		return fmt.Errorf("'values' array cannot be empty")
	}

	if weights, ok := params["weights"].([]any); ok {
		if len(weights) != len(valSlice) {
			return fmt.Errorf("'weights' length (%d) must match 'values' length (%d)", len(weights), len(valSlice))
		}
		for i, w := range weights {
			if _, ok := w.(float64); !ok {
				if _, ok := w.(int); !ok {
					return fmt.Errorf("weights[%d] must be a number", i)
				}
			}
		}
	}
	return nil
}

func (g *RangeGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	values := params["values"].([]any)
	if weights, ok := params["weights"].([]any); ok {
		return g.pickWeighted(values, weights), nil
	}
	return values[rand.Intn(len(values))], nil
}

func (g *RangeGenerator) pickWeighted(values, weightsRaw []any) any {
	weights := make([]float64, len(weightsRaw))
	for i, w := range weightsRaw {
		switch val := w.(type) {
		case float64:
			weights[i] = val
		case int:
			weights[i] = float64(val)
		}
	}

	var total float64
	for _, w := range weights {
		total += w
	}

	r, sum := rand.Float64()*total, 0.0
	for i, w := range weights {
		sum += w
		if r <= sum {
			return values[i]
		}
	}
	return values[len(values)-1]
}
