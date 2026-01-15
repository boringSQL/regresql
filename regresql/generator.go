package regresql

import (
	"fmt"
	"sync"
)

type (
	// Generator generates data for column
	Generator interface {
		// Generate produces a value for the column based on params
		Generate(params map[string]any, column *ColumnInfo) (any, error)

		// Name returns the generator's unique name
		Name() string

		// Validate checks if params are valid for this generator
		Validate(params map[string]any, column *ColumnInfo) error
	}

	// GeneratorRegistry manages available generators
	GeneratorRegistry struct {
		generators map[string]Generator
		mu         sync.RWMutex
	}

	// BaseGenerator provides common functionality for generators
	BaseGenerator struct {
		name string
	}
)

// NewGeneratorRegistry creates a new generator registry
func NewGeneratorRegistry() *GeneratorRegistry {
	return &GeneratorRegistry{
		generators: make(map[string]Generator),
	}
}

// Register adds a generator to the registry
func (gr *GeneratorRegistry) Register(gen Generator) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	name := gen.Name()
	if name == "" {
		return fmt.Errorf("generator name cannot be empty")
	}

	if _, exists := gr.generators[name]; exists {
		return fmt.Errorf("generator '%s' already registered", name)
	}

	gr.generators[name] = gen
	return nil
}

// Get retrieves a generator by name
func (gr *GeneratorRegistry) Get(name string) (Generator, error) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	gen, exists := gr.generators[name]
	if !exists {
		return nil, ErrGeneratorNotFound(name)
	}

	return gen, nil
}

// List returns all registered generator names
func (gr *GeneratorRegistry) List() []string {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	names := make([]string, 0, len(gr.generators))
	for name := range gr.generators {
		names = append(names, name)
	}

	return names
}

// Has checks if a generator is registered
func (gr *GeneratorRegistry) Has(name string) bool {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	_, exists := gr.generators[name]
	return exists
}

// Unregister removes a generator from the registry
func (gr *GeneratorRegistry) Unregister(name string) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, exists := gr.generators[name]; !exists {
		return ErrGeneratorNotFound(name)
	}

	delete(gr.generators, name)
	return nil
}

// Clear removes all generators from the registry
func (gr *GeneratorRegistry) Clear() {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	gr.generators = make(map[string]Generator)
}

// Name returns the generator's name
func (bg *BaseGenerator) Name() string {
	return bg.name
}

// getParam retrieves a parameter value with type assertion and numeric conversion
func getParam[T any](params map[string]any, key string, defaultValue T) T {
	val, exists := params[key]
	if !exists {
		return defaultValue
	}

	// Direct type assertion
	if typed, ok := val.(T); ok {
		return typed
	}

	// Handle numeric type conversions (YAML parses numbers as int, but we often want int64)
	var result any
	switch any(defaultValue).(type) {
	case int64:
		switch v := val.(type) {
		case int:
			result = int64(v)
		case int64:
			result = v
		case float64:
			result = int64(v)
		}
	case int:
		switch v := val.(type) {
		case int:
			result = v
		case int64:
			result = int(v)
		case float64:
			result = int(v)
		}
	case float64:
		switch v := val.(type) {
		case int:
			result = float64(v)
		case int64:
			result = float64(v)
		case float64:
			result = v
		}
	}

	if result != nil {
		if typed, ok := result.(T); ok {
			return typed
		}
	}

	return defaultValue
}

// getRequiredParam retrieves a required parameter or returns an error
func getRequiredParam[T any](params map[string]any, key string) (T, error) {
	var zero T
	val, exists := params[key]
	if !exists {
		return zero, fmt.Errorf("required parameter '%s' not found", key)
	}

	typed, ok := val.(T)
	if !ok {
		return zero, fmt.Errorf("parameter '%s' has wrong type", key)
	}

	return typed, nil
}
