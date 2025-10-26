package regresql

import (
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
)

type ForeignKeyGenerator struct {
	db      *sql.DB
	cache   map[string][]interface{}
	cacheMu sync.RWMutex
	nextIdx map[string]int
	idxMu   sync.Mutex
}

func NewForeignKeyGenerator(db *sql.DB) *ForeignKeyGenerator {
	return &ForeignKeyGenerator{
		db:      db,
		cache:   make(map[string][]interface{}),
		nextIdx: make(map[string]int),
	}
}

func (g *ForeignKeyGenerator) Name() string { return "fk" }

func (g *ForeignKeyGenerator) Validate(params map[string]interface{}, column *ColumnInfo) error {
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

	return nil
}

func (g *ForeignKeyGenerator) Generate(params map[string]interface{}, column *ColumnInfo) (interface{}, error) {
	table, col := params["table"].(string), params["column"].(string)
	strategy, _ := params["strategy"].(string)
	if strategy == "" {
		strategy = "random"
	}

	values, err := g.getAvailableValues(table, col)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("no rows in %s.%s to reference", table, col)
	}

	switch strategy {
	case "sequential":
		return g.pickSequential(table, col, values), nil
	case "weighted":
		return g.pickWeighted(values, g.extractWeights(params, len(values))), nil
	default:
		return values[rand.Intn(len(values))], nil
	}
}

func (g *ForeignKeyGenerator) getAvailableValues(table, column string) ([]interface{}, error) {
	key := table + "." + column

	g.cacheMu.RLock()
	if cached, ok := g.cache[key]; ok {
		g.cacheMu.RUnlock()
		return cached, nil
	}
	g.cacheMu.RUnlock()

	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s", column, table, column)
	rows, err := g.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s.%s: %w", table, column, err)
	}
	defer rows.Close()

	var values []interface{}
	for rows.Next() {
		var val interface{}
		if err := rows.Scan(&val); err != nil {
			return nil, fmt.Errorf("failed to scan %s.%s: %w", table, column, err)
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

func (g *ForeignKeyGenerator) pickSequential(table, column string, values []interface{}) interface{} {
	key := table + "." + column

	g.idxMu.Lock()
	idx := g.nextIdx[key]
	g.nextIdx[key] = (idx + 1) % len(values)
	g.idxMu.Unlock()

	return values[idx]
}

func (g *ForeignKeyGenerator) extractWeights(params map[string]interface{}, count int) []float64 {
	if w, ok := params["weights"].([]interface{}); ok {
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

func (g *ForeignKeyGenerator) pickWeighted(values []interface{}, weights []float64) interface{} {
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
	g.cache = make(map[string][]interface{})
	g.cacheMu.Unlock()

	g.idxMu.Lock()
	g.nextIdx = make(map[string]int)
	g.idxMu.Unlock()
}
