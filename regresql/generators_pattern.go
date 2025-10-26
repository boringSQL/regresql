package regresql

import (
	"fmt"
	"regexp"
	"strings"
)

type PatternGenerator struct {
	registry *GeneratorRegistry
}

func NewPatternGenerator(registry *GeneratorRegistry) *PatternGenerator {
	return &PatternGenerator{registry: registry}
}

func (g *PatternGenerator) Name() string { return "pattern" }

func (g *PatternGenerator) Validate(params map[string]interface{}, column *ColumnInfo) error {
	if _, ok := params["template"]; !ok {
		return fmt.Errorf("pattern generator requires 'template' parameter")
	}

	template := params["template"].(string)
	placeholders := g.extractPlaceholders(template)

	subParams, ok := params["params"].(map[string]interface{})
	if !ok && len(placeholders) > 0 {
		return fmt.Errorf("pattern generator requires 'params' map for placeholders")
	}

	for _, ph := range placeholders {
		spec, ok := subParams[ph]
		if !ok {
			return fmt.Errorf("missing params definition for placeholder '%s'", ph)
		}

		specMap, ok := spec.(map[string]interface{})
		if !ok {
			return fmt.Errorf("params.%s must be a map", ph)
		}

		genName, ok := specMap["generator"].(string)
		if !ok {
			return fmt.Errorf("params.%s missing 'generator' field", ph)
		}

		if !g.registry.Has(genName) {
			return fmt.Errorf("unknown generator '%s' for placeholder '%s'", genName, ph)
		}
	}

	return nil
}

func (g *PatternGenerator) Generate(params map[string]interface{}, column *ColumnInfo) (interface{}, error) {
	tmpl := params["template"].(string)
	placeholders := g.extractPlaceholders(tmpl)
	if len(placeholders) == 0 {
		return tmpl, nil
	}

	subParams := params["params"].(map[string]interface{})
	result := tmpl

	for _, ph := range placeholders {
		spec := subParams[ph].(map[string]interface{})
		gen, err := g.registry.Get(spec["generator"].(string))
		if err != nil {
			return nil, fmt.Errorf("generator '%s' not found: %w", spec["generator"], err)
		}

		value, err := gen.Generate(spec, column)
		if err != nil {
			return nil, fmt.Errorf("failed to generate value for '%s': %w", ph, err)
		}

		result = strings.ReplaceAll(result, "{{"+ph+"}}", fmt.Sprint(value))
	}
	return result, nil
}

func (g *PatternGenerator) extractPlaceholders(template string) []string {
	re := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	matches := re.FindAllStringSubmatch(template, -1)

	seen := make(map[string]bool)
	var placeholders []string
	for _, match := range matches {
		if len(match) > 1 {
			ph := strings.TrimSpace(match[1])
			if !seen[ph] {
				placeholders = append(placeholders, ph)
				seen[ph] = true
			}
		}
	}

	return placeholders
}
