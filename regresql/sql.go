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
		NoTest             bool
		NoBaseline         bool
		NoSeqScanWarn      bool
		DiffFloatTolerance float64
	}
)

func (q *Query) GetRegressQLOptions() RegressQLOptions {
	opts := RegressQLOptions{}
	metadata, ok := q.GetMetadata("regresql")
	if !ok {
		return opts
	}

	for _, part := range strings.Split(metadata, ",") {
		part = strings.TrimSpace(part)
		partLower := strings.ToLower(part)

		switch {
		case partLower == "notest":
			opts.NoTest = true
		case partLower == "nobaseline":
			opts.NoBaseline = true
		case partLower == "noseqscanwarn":
			opts.NoSeqScanWarn = true
		case strings.HasPrefix(partLower, "difffloattolerance:"):
			// Parse DiffFloatTolerance:0.01
			value := strings.TrimPrefix(part, "DiffFloatTolerance:")
			value = strings.TrimPrefix(value, "difffloattolerance:")
			fmt.Sscanf(value, "%f", &opts.DiffFloatTolerance)
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

func (q *Query) Prepare(bindings map[string]string) (string, []any) {
	params := make([]any, len(q.Args))
	for i, varname := range q.Args {
		params[i] = bindings[varname]
	}
	return q.OrdinalQuery, params
}
