package regresql

import (
	"fmt"
	"strings"
	"time"

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
		NoGeneric          bool
		DiffFloatTolerance float64
		Timeout            time.Duration // statement_timeout override (0 = unset)
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
		case partLower == "nogeneric":
			opts.NoGeneric = true
		case strings.HasPrefix(partLower, "difffloattolerance:"):
			// Parse DiffFloatTolerance:0.01
			value := strings.TrimPrefix(part, "DiffFloatTolerance:")
			value = strings.TrimPrefix(value, "difffloattolerance:")
			fmt.Sscanf(value, "%f", &opts.DiffFloatTolerance)
		case strings.HasPrefix(partLower, "timeout:"):
			value := part[len("timeout:"):]
			if d, err := time.ParseDuration(strings.TrimSpace(value)); err == nil {
				opts.Timeout = d
			}
		}
	}

	return opts
}

func (q *Query) uniqueArgs() []string {
	seen := make(map[string]bool)
	args := []string{}
	for _, varname := range q.Args {
		if !seen[varname] {
			seen[varname] = true
			args = append(args, varname)
		}
	}
	return args
}

// GetParamTypes returns the types from `-- paramtypes: a, b`, one per unique
// named argument in order of first appearance. nil means the query has none.
func (q *Query) GetParamTypes() ([]string, error) {
	metadata, ok := q.GetMetadata("paramtypes")
	if !ok {
		return nil, nil
	}

	types := splitTopLevel(metadata)

	if want := len(q.uniqueArgs()); len(types) != want {
		return nil, fmt.Errorf("paramtypes lists %d types, query has %d parameters", len(types), want)
	}
	return types, nil
}

// splitTopLevel splits on commas outside parentheses, so numeric(10,2) stays whole.
func splitTopLevel(s string) []string {
	var parts []string
	depth, start := 0, 0
	add := func(end int) {
		if part := strings.TrimSpace(s[start:end]); part != "" {
			parts = append(parts, part)
		}
	}
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				add(i)
				start = i + 1
			}
		}
	}
	add(len(s))
	return parts
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
	q, err := queries.NewQuery(name, "", sqlText, nil)
	if err != nil {
		return nil, err
	}
	return &Query{Query: q}, nil
}

func (q *Query) Prepare(bindings map[string]any) (string, []any) {
	uniqueArgs := q.uniqueArgs()
	params := make([]any, len(uniqueArgs))
	for i, varname := range uniqueArgs {
		params[i] = bindings[varname]
	}
	return q.OrdinalQuery, params
}
