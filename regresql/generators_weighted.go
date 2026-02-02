package regresql

import (
	"fmt"
	"math/rand"
	"sort"
)

// WeightedGenerator generates values based on weighted probabilities
type WeightedGenerator struct {
	BaseGenerator
}

func NewWeightedGenerator() *WeightedGenerator {
	return &WeightedGenerator{
		BaseGenerator: BaseGenerator{name: "weighted"},
	}
}

func (g *WeightedGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	nullProb := getParam(params, "null_probability", 0.0)

	// Handle null probability
	if nullProb > 0 && rand.Float64() < nullProb {
		return nil, nil
	}

	// Get values map
	valuesRaw, ok := params["values"]
	if !ok {
		return nil, fmt.Errorf("weighted generator requires 'values' parameter")
	}

	// Parse values into weighted list
	weights, err := parseWeightedValues(valuesRaw)
	if err != nil {
		return nil, err
	}

	if len(weights) == 0 {
		return nil, fmt.Errorf("weighted generator: no values provided")
	}

	// Calculate total weight
	totalWeight := 0
	for _, w := range weights {
		totalWeight += w.weight
	}

	// Pick random value based on weights
	r := rand.Intn(totalWeight)
	cumulative := 0
	for _, w := range weights {
		cumulative += w.weight
		if r < cumulative {
			return w.value, nil
		}
	}

	// Fallback to last value
	return weights[len(weights)-1].value, nil
}

func (g *WeightedGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	valuesRaw, ok := params["values"]
	if !ok {
		return fmt.Errorf("weighted generator requires 'values' parameter")
	}

	weights, err := parseWeightedValues(valuesRaw)
	if err != nil {
		return err
	}

	if len(weights) == 0 {
		return fmt.Errorf("weighted generator: at least one value required")
	}

	for _, w := range weights {
		if w.weight < 0 {
			return fmt.Errorf("weighted generator: weights must be non-negative")
		}
	}

	nullProb := getParam(params, "null_probability", 0.0)
	if nullProb < 0 || nullProb > 1 {
		return fmt.Errorf("null_probability must be between 0 and 1")
	}

	return nil
}

type weightedValue struct {
	value  string
	weight int
}

func parseWeightedValues(raw any) ([]weightedValue, error) {
	var result []weightedValue

	switch v := raw.(type) {
	case map[string]any:
		// Map format: {"value1": weight1, "value2": weight2}
		for key, val := range v {
			weight, err := toInt(val)
			if err != nil {
				return nil, fmt.Errorf("invalid weight for %q: %w", key, err)
			}
			result = append(result, weightedValue{value: key, weight: weight})
		}
	case map[string]int:
		// Already typed map
		for key, weight := range v {
			result = append(result, weightedValue{value: key, weight: weight})
		}
	case map[any]any:
		// YAML may produce this for numeric keys - convert keys to strings
		for key, val := range v {
			keyStr := fmt.Sprintf("%v", key)
			weight, err := toInt(val)
			if err != nil {
				return nil, fmt.Errorf("invalid weight for %q: %w", keyStr, err)
			}
			result = append(result, weightedValue{value: keyStr, weight: weight})
		}
	case []any:
		// Array of {value, freq} objects
		for i, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("values[%d]: expected object with 'value' and 'freq'", i)
			}
			val, ok := obj["value"].(string)
			if !ok {
				return nil, fmt.Errorf("values[%d]: 'value' must be a string", i)
			}
			freq, err := toFloat(obj["freq"])
			if err != nil {
				return nil, fmt.Errorf("values[%d]: invalid 'freq': %w", i, err)
			}
			// Convert frequency (0-1) to weight (percentage)
			weight := int(freq * 100)
			if weight < 1 {
				weight = 1
			}
			result = append(result, weightedValue{value: val, weight: weight})
		}
	default:
		return nil, fmt.Errorf("values must be a map or array, got %T", raw)
	}

	// Sort for deterministic behavior
	sort.Slice(result, func(i, j int) bool {
		if result[i].weight != result[j].weight {
			return result[i].weight > result[j].weight // Higher weight first
		}
		return result[i].value < result[j].value
	})

	return result, nil
}

func toInt(v any) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case float64:
		return int(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func toFloat(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}
