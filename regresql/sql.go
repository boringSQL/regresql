package regresql

import (
	"fmt"

	"github.com/boringsql/queries"
)

/*

A query instances represents an SQL query, read from Path filename and
stored raw as the Text slot. The query text is "parsed" into the Query slot,
and parameters are extracted into both the Vars slot and the Params slot.

    SELECT * FROM foo WHERE a = :a and b between :a and :b;

In the previous query, we would have Vars = [a b] and Params = [a a b].

This struct now embeds github.com/boringsql/queries.Query which provides
all the parsing and parameter handling functionality.
*/
type Query struct {
	*queries.Query
}

// Parse a SQL file and returns map of Queries instances, with variables
// used in the query separated in the Query.Vars map.
func parseQueryFile(queryPath string) (map[string]*Query, error) {
	store := queries.NewQueryStore()
	err := store.LoadFromFile(queryPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open query file '%s': %s\n", queryPath, err.Error())
	}

	result := make(map[string]*Query)

	for name, bqQuery := range store.Queries() {
		// Skip the "default" query if it's empty (queries without names)
		if name == "default" && bqQuery.RawQuery() == "" {
			continue
		}

		result[name] = &Query{Query: bqQuery}
	}

	return result, nil
}

// Prepare an args... interface{} for Query from given bindings
func (q *Query) Prepare(bindings map[string]string) (string, []interface{}) {
	// Build ordered params array matching q.Args (all occurrences with duplicates)
	params := make([]interface{}, len(q.Args))
	for i, varname := range q.Args {
		params[i] = bindings[varname]
	}

	return q.OrdinalQuery, params
}
