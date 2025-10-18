package regresql

import (
	"fmt"
	"strings"

	"github.com/boringsql/queries"
)

type (
	Query struct {
		*queries.Query
	}

	RegressQLOptions struct {
		NoTest     bool
		NoBaseline bool
	}
)

func (q *Query) GetRegressQLOptions() RegressQLOptions {
	opts := RegressQLOptions{}
	metadata, ok := q.GetMetadata("regresql")
	if !ok {
		return opts
	}

	for _, part := range strings.Split(metadata, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "notest":
			opts.NoTest = true
		case "nobaseline":
			opts.NoBaseline = true
		}
	}

	return opts
}

func parseQueryFile(queryPath string) (map[string]*Query, error) {
	store := queries.NewQueryStore()
	if err := store.LoadFromFile(queryPath); err != nil {
		return nil, fmt.Errorf("failed to open query file '%s': %w", queryPath, err)
	}

	result := make(map[string]*Query)
	for name, bqQuery := range store.Queries() {
		if name == "default" && bqQuery.RawQuery() == "" {
			continue
		}
		result[name] = &Query{Query: bqQuery}
	}

	return result, nil
}

func NewQueryFromString(name, sqlText string) (*Query, error) {
	return &Query{Query: queries.NewQuery(name, "", sqlText, nil)}, nil
}

func (q *Query) Prepare(bindings map[string]string) (string, []interface{}) {
	params := make([]interface{}, len(q.Args))
	for i, varname := range q.Args {
		params[i] = bindings[varname]
	}
	return q.OrdinalQuery, params
}
