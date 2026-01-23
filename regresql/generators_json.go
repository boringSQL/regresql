package regresql

import "fmt"

type JSONGenerator struct {
	registry *GeneratorRegistry
}

func NewJSONGenerator(registry *GeneratorRegistry) *JSONGenerator {
	return &JSONGenerator{registry: registry}
}

func (g *JSONGenerator) Name() string { return "json" }

func (g *JSONGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	_, hasSchema := params["schema"]
	_, hasValue := params["value"]

	if !hasSchema && !hasValue {
		return fmt.Errorf("json generator requires either 'schema' or 'value' parameter")
	}
	if hasSchema && hasValue {
		return fmt.Errorf("json generator cannot have both 'schema' and 'value'")
	}

	if hasSchema {
		schema, ok := params["schema"].(map[string]any)
		if !ok {
			return fmt.Errorf("'schema' must be an object")
		}
		return g.validateSchema(schema)
	}
	return nil
}

func (g *JSONGenerator) validateSchema(schema map[string]any) error {
	for field, spec := range schema {
		specMap, ok := spec.(map[string]any)
		if !ok {
			continue // static value
		}

		genName, ok := specMap["generator"].(string)
		if !ok {
			continue // static value (object without generator key)
		}

		gen, err := g.registry.Get(genName)
		if err != nil {
			return fmt.Errorf("field '%s': %w", field, err)
		}

		if err := gen.Validate(specMap, nil); err != nil {
			return fmt.Errorf("field '%s': %w", field, err)
		}
	}
	return nil
}

func (g *JSONGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	// Static value mode
	if value, ok := params["value"]; ok {
		return value, nil
	}

	// Schema mode
	schema := params["schema"].(map[string]any)
	result := make(map[string]any)

	for field, spec := range schema {
		specMap, ok := spec.(map[string]any)
		if !ok {
			result[field] = spec
			continue
		}

		genName, ok := specMap["generator"].(string)
		if !ok {
			result[field] = spec
			continue
		}

		gen, err := g.registry.Get(genName)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field, err)
		}

		value, err := gen.Generate(specMap, nil)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field, err)
		}
		result[field] = value
	}

	return result, nil
}
