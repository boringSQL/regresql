package regresql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// admitPerturbations change the PLAN but must never change a correct query's
// RESULT; divergence under any = nondeterministic, reject.
var admitPerturbations = []struct {
	name string
	sets []string
}{
	{"baseline", nil},
	{"no_seqscan", []string{"SET enable_seqscan=off"}},
	{"no_indexscan", []string{"SET enable_indexscan=off", "SET enable_indexonlyscan=off", "SET enable_bitmapscan=off"}},
	{"no_hashjoin", []string{"SET enable_hashjoin=off"}},
	{"no_mergejoin", []string{"SET enable_mergejoin=off"}},
	{"no_nestloop", []string{"SET enable_nestloop=off"}},
	{"no_hashagg", []string{"SET enable_hashagg=off"}},
	{"parallel", []string{"SET max_parallel_workers_per_gather=4", "SET parallel_setup_cost=0", "SET parallel_tuple_cost=0", "SET min_parallel_table_scan_size=0", "SET min_parallel_index_scan_size=0"}},
	{"tiny_workmem", []string{"SET work_mem='64kB'"}},
}

// admitPrelude resets each session to a clean serial state. RESET ALL first:
// pooled connections carry SETs from prior calls, which would mask the
// nondeterminism we test for. The timeout caps a perturbation-forced bad plan.
var admitPrelude = []string{
	"RESET ALL",
	"SET max_parallel_workers_per_gather=0",
	"SET jit=off",
	"SET statement_timeout='20s'",
}

// DefaultAdmitReps: >1 catches run-to-run nondeterminism (e.g. parallel Top-N
// breaking ties differently each run).
const DefaultAdmitReps = 3

type (
	AdmitOptions struct {
		Root       string
		URI        string
		RunFilter  string
		Format     string // console | json
		OutputPath string
		Reps       int
		Strict     bool // exit 1 if any query is rejected
	}

	AdmitResult struct {
		Name     string `json:"name"`
		Binding  string `json:"binding,omitempty"`
		Admitted bool   `json:"admitted"`
		Reason   string `json:"reason,omitempty"`
	}
)

func Admit(opts AdmitOptions) int {
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
	reps := opts.Reps
	if reps < 1 {
		reps = DefaultAdmitReps
	}

	db, err := openCompareDB(opts.URI) // simple protocol so the parallel perturbation engages
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

	var results []AdmitResult
	for _, pq := range plannedQueries {
		if !suite.matchesRunFilter(filepath.Base(pq.SQLPath), pq.Query.Name) {
			continue
		}
		if pq.Query.GetRegressQLOptions().NoTest {
			continue
		}
		for _, b := range iterateBindings(pq.Plan) {
			results = append(results, admitBinding(context.Background(), db, pq.Query, b, reps))
		}
	}

	if err := renderAdmit(results, opts.Format, opts.OutputPath); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 2
	}
	if opts.Strict {
		for _, r := range results {
			if !r.Admitted {
				return 1
			}
		}
	}
	return 0
}

// admitBinding hashes the query under each perturbation via the live connection.
func admitBinding(ctx context.Context, db *sql.DB, q *Query, b bindingRef, reps int) AdmitResult {
	res := AdmitResult{Name: q.Name, Binding: b.name}

	sqlText := q.OrdinalQuery
	var args []any
	if len(q.Args) > 0 {
		sqlText, args = q.Prepare(b.bindings)
	}

	res.Admitted, res.Reason = admitDecision(reps, func(sets []string) (string, error) {
		return canonicalResultHash(ctx, db, sqlText, args, sets)
	})
	return res
}

// admitDecision hashes the baseline, then every perturbation reps times via hash;
// admitted iff every hash matches the baseline. First divergence/error wins.
func admitDecision(reps int, hash func(sets []string) (string, error)) (bool, string) {
	base, err := hash(nil)
	if err != nil {
		return false, "baseline error: " + admitOneline(err.Error())
	}
	for _, p := range admitPerturbations {
		for r := 0; r < reps; r++ {
			h, err := hash(p.sets)
			if err != nil {
				return false, fmt.Sprintf("%s error: %s", p.name, admitOneline(err.Error()))
			}
			if h != base {
				return false, "result not invariant under " + p.name
			}
		}
	}
	return true, ""
}

// canonicalResultHash returns an md5 of the result as a SORTED multiset, so mere
// row reordering hashes identically; only a content change moves it.
func canonicalResultHash(ctx context.Context, db *sql.DB, sqlText string, args []any, sets []string) (string, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	for _, s := range append(append([]string(nil), admitPrelude...), sets...) {
		if _, err := conn.ExecContext(ctx, s); err != nil {
			return "", fmt.Errorf("%s: %w", s, err)
		}
	}

	inner := strings.TrimRight(strings.TrimSpace(sqlText), ";")
	wrapped := fmt.Sprintf(
		"SELECT md5(coalesce(string_agg(r, chr(10) ORDER BY r), '')) "+
			"FROM (SELECT _row::text AS r FROM (%s) _row) s", inner)
	var h string
	if err := conn.QueryRowContext(ctx, wrapped, args...).Scan(&h); err != nil {
		return "", err
	}
	return h, nil
}

func renderAdmit(results []AdmitResult, format, outputPath string) error {
	w, closeFn, err := getWriter(outputPath)
	if err != nil {
		return err
	}
	defer closeFn()

	admitted := 0
	for _, r := range results {
		if r.Admitted {
			admitted++
		}
	}

	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"admitted": admitted,
			"total":    len(results),
			"results":  results,
		})
	}

	fmt.Fprintln(w, "== Admission (determinism filter) ==")
	for _, r := range results {
		label := r.Name
		if r.Binding != "" {
			label = r.Name + " (" + r.Binding + ")"
		}
		if r.Admitted {
			fmt.Fprintf(w, "  ADMIT   %s\n", label)
		} else {
			fmt.Fprintf(w, "  REJECT  %-28s %s\n", label, r.Reason)
		}
	}
	fmt.Fprintf(w, "\n  admitted %d / %d\n", admitted, len(results))
	return nil
}

func admitOneline(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 90 {
		s = s[:90] + "…"
	}
	return s
}
