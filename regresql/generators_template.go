package regresql

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type TemplateGenerator struct {
	registry *GeneratorRegistry
}

func NewTemplateGenerator(registry *GeneratorRegistry) *TemplateGenerator {
	return &TemplateGenerator{registry: registry}
}

func (g *TemplateGenerator) Name() string { return "template" }

func (g *TemplateGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	if _, ok := params["template"]; !ok {
		return fmt.Errorf("template generator requires 'template' parameter")
	}

	tmplStr := params["template"].(string)
	tmpl := template.New("").Funcs(g.funcMap())

	if _, err := tmpl.Parse(tmplStr); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	if context, ok := params["context"].(map[string]any); ok {
		for key, spec := range context {
			specMap, ok := spec.(map[string]any)
			if !ok {
				return fmt.Errorf("context.%s must be a map", key)
			}

			genName, ok := specMap["generator"].(string)
			if !ok {
				return fmt.Errorf("context.%s missing 'generator' field", key)
			}

			if !g.registry.Has(genName) {
				return fmt.Errorf("unknown generator '%s' for context '%s'", genName, key)
			}
		}
	}

	return nil
}

func (g *TemplateGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	tmplStr := params["template"].(string)

	data := make(map[string]any)
	if context, ok := params["context"].(map[string]any); ok {
		for key, spec := range context {
			specMap := spec.(map[string]any)
			genName := specMap["generator"].(string)

			gen, err := g.registry.Get(genName)
			if err != nil {
				return nil, fmt.Errorf("generator '%s' not found: %w", genName, err)
			}

			value, err := gen.Generate(specMap, column)
			if err != nil {
				return nil, fmt.Errorf("failed to generate context '%s': %w", key, err)
			}

			data[key] = value
		}
	}

	tmpl := template.Must(template.New("").Funcs(g.funcMap()).Parse(tmplStr))

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func (g *TemplateGenerator) funcMap() template.FuncMap {
	return template.FuncMap{
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"title":     cases.Title(language.English).String,
		"trim":      strings.TrimSpace,
		"replace":   strings.ReplaceAll,
		"substr":    substr,
		"random":    randomInt,
		"uuid":      func() string { return uuid.New().String() },
		"now":       func() string { return time.Now().Format(time.RFC3339) },
		"date":      func(format string) string { return time.Now().Format(format) },
		"randStr":   randomString,
		"choice":    choice,
		"join":      strings.Join,
		"split":     strings.Split,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
	}
}

func substr(s string, start, length int) string {
	if start < 0 || start >= len(s) {
		return ""
	}
	if end := start + length; end <= len(s) {
		return s[start:end]
	}
	return s[start:]
}

func randomInt(min, max int) int {
	if min >= max {
		return min
	}
	return min + rand.Intn(max-min)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

func choice(items ...any) any {
	if len(items) == 0 {
		return ""
	}
	return items[rand.Intn(len(items))]
}
