package regresql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// These are pure optimizations: turning them off may change the plan but must
// never change the rows. If it does, the optimizer has a wrong-results bug.
var metamorphicGUCs = []string{
	"enable_eager_aggregate",
	"enable_memoize",
	"enable_incremental_sort",
	"enable_partitionwise_join",
	"enable_partitionwise_aggregate",
}

type (
	MetamorphicOptions struct {
		Root       string
		URI        string
		RunFilter  string
		Format     string
		OutputPath string
	}

	MetamorphicResult struct {
		Name    string `json:"name"`
		Binding string `json:"binding,omitempty"`
		Bug     bool   `json:"bug"`
		GUC     string `json:"guc,omitempty"`
		Reason  string `json:"reason,omitempty"`
	}
)

func Metamorphic(opts MetamorphicOptions) int {
	if config, err := ReadConfig(opts.Root); err == nil {
		SetGlobalConfig(config)
		if opts.URI == "" {
			opts.URI = config.PgUri
		}
	}
	if opts.URI == "" {
		fmt.Fprintln(os.Stderr, "no connection string (pass --uri or set pguri in config)")
		return 2
	}

	db, err := openCompareDB(opts.URI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 2
	}
	defer db.Close()

	plannedQueries, err := WalkPlans(opts.Root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to walk plans: %s\n", err)
		return 2
	}
	suite := Walk(opts.Root, nil)
	suite.SetRunFilter(opts.RunFilter)

	var results []MetamorphicResult
	for _, pq := range plannedQueries {
		if !suite.matchesRunFilter(filepath.Base(pq.SQLPath), pq.Query.Name) {
			continue
		}
		if pq.Query.GetRegressQLOptions().NoTest {
			continue
		}
		for _, b := range iterateBindings(pq.Plan) {
			results = append(results, metamorphicCheck(context.Background(), db, pq.Query, b))
		}
	}

	renderMetamorphic(results, opts.Format, opts.OutputPath)

	for _, r := range results {
		if r.Bug {
			return 1
		}
	}
	return 0
}

// metamorphicCheck hashes the query at its default plan, then flips each
// optimization off and checks the result didn't move. Assumes the query is
// deterministic to begin with (run `admit` first), otherwise a plan-dependent
// result would look like a bug here.
func metamorphicCheck(ctx context.Context, db *sql.DB, q *Query, b bindingRef) MetamorphicResult {
	res := MetamorphicResult{Name: q.Name, Binding: b.name}

	sqlText := q.OrdinalQuery
	var args []any
	if len(q.Args) > 0 {
		sqlText, args = q.Prepare(b.bindings)
	}

	base, err := canonicalResultHash(ctx, db, sqlText, args, nil)
	if err != nil {
		res.Reason = admitOneline(err.Error())
		return res
	}
	for _, guc := range metamorphicGUCs {
		h, err := canonicalResultHash(ctx, db, sqlText, args, []string{"SET " + guc + "=off"})
		if err != nil {
			// GUC not known on this PG version, skip it
			continue
		}
		if h != base {
			res.Bug = true
			res.GUC = guc
			return res
		}
	}
	return res
}

func renderMetamorphic(results []MetamorphicResult, format, outputPath string) {
	w, closeFn, err := getWriter(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}
	defer closeFn()

	bugs := 0
	for _, r := range results {
		if r.Bug {
			bugs++
		}
	}

	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"bugs": bugs, "total": len(results), "results": results})
		return
	}

	fmt.Fprintln(w, "== Metamorphic oracle (result-preserving optimizations) ==")
	for _, r := range results {
		label := r.Name
		if r.Binding != "" {
			label = r.Name + " (" + r.Binding + ")"
		}
		switch {
		case r.Bug:
			fmt.Fprintf(w, "  BUG     %-28s result changed with %s=off\n", label, r.GUC)
		case r.Reason != "":
			fmt.Fprintf(w, "  error   %-28s %s\n", label, r.Reason)
		default:
			fmt.Fprintf(w, "  ok      %s\n", label)
		}
	}
	fmt.Fprintf(w, "\n  %d checked, %d wrong-results bugs\n", len(results), bugs)
}
