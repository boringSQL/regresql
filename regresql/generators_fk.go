package regresql

import (
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"sync"
)

type (
	// Querier interface allows querying from either *sql.DB or *sql.Tx

	Querier interface {
		Query(query string, args ...any) (*sql.Rows, error)
	}

	ForeignKeyGenerator struct {
		db      *sql.DB
		querier Querier // Can be set to tx during fixture application
		cache   map[string][]any
		cacheMu sync.RWMutex
		nextIdx map[string]int
		idxMu   sync.Mutex
	}
)

func NewForeignKeyGenerator(db *sql.DB) *ForeignKeyGenerator {
	return &ForeignKeyGenerator{
		db:      db,
		querier: db, // Default to db
		cache:   make(map[string][]any),
		nextIdx: make(map[string]int),
	}
}

// SetQuerier sets the querier to use (typically set to tx during fixture application)
func (g *ForeignKeyGenerator) SetQuerier(q Querier) {
	g.querier = q
	// Clear cache when switching queriers to avoid stale data
	g.ClearCache()
}

func (g *ForeignKeyGenerator) Name() string { return "fk" }

func (g *ForeignKeyGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	if _, ok := params["table"]; !ok {
		return fmt.Errorf("fk generator requires 'table' parameter")
	}
	if _, ok := params["column"]; !ok {
		return fmt.Errorf("fk generator requires 'column' parameter")
	}

	if s, ok := params["strategy"].(string); ok {
		validStrategies := map[string]bool{"random": true, "sequential": true, "weighted": true}
		if !validStrategies[s] {
			return fmt.Errorf("invalid strategy '%s', must be one of: random, sequential, weighted", s)
		}
	}

	// Validate values parameter if provided
	if values, ok := params["values"]; ok {
		if _, ok := values.([]any); !ok {
			return fmt.Errorf("'values' parameter must be an array")
		}
	}

	return nil
}

func (g *ForeignKeyGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	table := params["table"].(string)
	lookupCol := params["column"].(string)
	returnCol, _ := params["return_column"].(string)
	if returnCol == "" {
		returnCol = lookupCol
	}

	strategy, _ := params["strategy"].(string)
	if strategy == "" {
		strategy = "random"
	}

	// Get available values with optional filtering
	values, err := g.getAvailableValues(table, lookupCol, returnCol, params)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("no rows in %s.%s to reference", table, lookupCol)
	}

	// Use the combined key for sequential strategy
	cacheKey := fmt.Sprintf("%s.%s->%s", table, lookupCol, returnCol)

	switch strategy {
	case "sequential":
		return g.pickSequential(cacheKey, values), nil
	case "weighted":
		return g.pickWeighted(values, g.extractWeights(params, len(values))), nil
	default:
		return values[rand.Intn(len(values))], nil
	}
}

func (g *ForeignKeyGenerator) getAvailableValues(table, lookupCol, returnCol string, params map[string]any) ([]any, error) {
	// Build cache key
	key := fmt.Sprintf("%s.%s->%s", table, lookupCol, returnCol)

	// Add values filter to cache key if present
	if filterValues, ok := params["values"].([]any); ok {
		key += fmt.Sprintf(":filtered(%d)", len(filterValues))
	}

	g.cacheMu.RLock()
	if cached, ok := g.cache[key]; ok {
		g.cacheMu.RUnlock()
		return cached, nil
	}
	g.cacheMu.RUnlock()

	// Build query with optional WHERE clause for values filter
	var query string
	var args []any

	if filterValues, ok := params["values"].([]any); ok {
		// Query with filter: SELECT returnCol FROM table WHERE lookupCol IN (...)
		placeholders := make([]string, len(filterValues))
		for i := range filterValues {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args = append(args, filterValues[i])
		}
		whereClause := fmt.Sprintf("%s IN (%s)", lookupCol, strings.Join(placeholders, ", "))
		query = fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s", returnCol, table, whereClause, returnCol)
	} else {
		// Query all: SELECT returnCol FROM table
		query = fmt.Sprintf("SELECT %s FROM %s ORDER BY %s", returnCol, table, returnCol)
	}

	rows, err := g.querier.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s.%s: %w", table, lookupCol, err)
	}
	defer rows.Close()

	var values []any
	for rows.Next() {
		var val any
		if err := rows.Scan(&val); err != nil {
			return nil, fmt.Errorf("failed to scan %s.%s: %w", table, returnCol, err)
		}
		values = append(values, val)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	g.cacheMu.Lock()
	g.cache[key] = values
	g.cacheMu.Unlock()

	return values, nil
}

func (g *ForeignKeyGenerator) pickSequential(cacheKey string, values []any) any {
	g.idxMu.Lock()
	idx := g.nextIdx[cacheKey]
	g.nextIdx[cacheKey] = (idx + 1) % len(values)
	g.idxMu.Unlock()

	return values[idx]
}

func (g *ForeignKeyGenerator) extractWeights(params map[string]any, count int) []float64 {
	if w, ok := params["weights"].([]any); ok {
		weights := make([]float64, len(w))
		for i, v := range w {
			switch val := v.(type) {
			case float64:
				weights[i] = val
			case int:
				weights[i] = float64(val)
			}
		}
		return weights
	}

	weights := make([]float64, count)
	for i := range weights {
		weights[i] = float64(i + 1)
	}
	return weights
}

func (g *ForeignKeyGenerator) pickWeighted(values []any, weights []float64) any {
	if len(weights) != len(values) {
		return values[rand.Intn(len(values))]
	}

	var total float64
	for _, w := range weights {
		total += w
	}

	r := rand.Float64() * total
	var sum float64
	for i, w := range weights {
		sum += w
		if r <= sum {
			return values[i]
		}
	}
	return values[len(values)-1]
}

func (g *ForeignKeyGenerator) ClearCache() {
	g.cacheMu.Lock()
	g.cache = make(map[string][]any)
	g.cacheMu.Unlock()

	g.idxMu.Lock()
	g.nextIdx = make(map[string]int)
	g.idxMu.Unlock()
}
