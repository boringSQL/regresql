package regresql

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// DefaultTieMarginPct: an alternative plan within this % of the chosen plan's
// cost is a tie -- the winner is decided by enumeration order, not the cost
// model, so it may differ between two instances of the SAME build.
const DefaultTieMarginPct = 1.0

// Perturbations used to discover the runner-up plan: each disables one path
// type and the cheapest resulting plan is the best alternative. Unknown GUCs
// are skipped, so this is safe across versions.
var costMarginGUCs = []string{
	"enable_hashjoin",
	"enable_mergejoin",
	"enable_nestloop",
	"enable_indexscan",
	"enable_indexonlyscan",
	"enable_bitmapscan",
	"enable_seqscan",
	"enable_sort",
	"enable_hashagg",
	"enable_material",
	"enable_memoize",
	"enable_partitionwise_join",
	"enable_incremental_sort",
}

// costMarginPass classifies cost ties DETERMINISTICALLY: it prices the
// runner-up on the same cost model, using EXPLAIN only (never ANALYZE), so for
// fixed statistics the tie set is a pure function of the cost model and does
// not move between runs. The re-ANALYZE stability pass instead answers whether
// ANALYZE's RNG moved the plan, so its tie set is itself random.
func costMarginPass(ctx context.Context, db *sql.DB, pqs []*PlannedQuery, suite *Suite, marginPct float64) map[string]string {
	if marginPct <= 0 {
		marginPct = DefaultTieMarginPct
	}
	ties := map[string]string{}

	for _, pq := range pqs {
		if !suite.matchesRunFilter(filepath.Base(pq.SQLPath), pq.Query.Name) {
			continue
		}
		if pq.Query.GetRegressQLOptions().NoTest {
			continue
		}
		for _, b := range iterateBindings(pq.Plan) {
			key := bindingKey(pq.Query.Name, b.name)
			margin, alt, ok := planCostMargin(ctx, db, pq.Query, b.bindings)
			if !ok {
				continue // could not plan it; leave it to the other filters
			}
			switch {
			case margin < 0:
				// the alternative prices BELOW the chosen plan: since PG18 an
				// enable_*=off minimizes disabled-node count before cost, so the
				// cheaper plan can lose. Not the cost model deciding this one.
				ties[key] = fmt.Sprintf(
					"cost tie: alternative plan (%s off) prices %.2f%% BELOW the chosen plan "+
						"-- the choice is not cost-driven", alt, -margin)
			case margin <= marginPct:
				ties[key] = fmt.Sprintf(
					"cost tie: runner-up plan (%s off) is within %.2f%% of the chosen plan",
					alt, margin)
			}
		}
	}
	return ties
}

// planCostMargin returns how much more expensive the best ALTERNATIVE plan is,
// as a percentage of the chosen plan's cost, plus the GUC that revealed it.
func planCostMargin(ctx context.Context, db *sql.DB, q *Query, bindings map[string]any) (float64, string, bool) {
	base, baseFP, ok := explainCostAndShape(ctx, db, q, bindings, nil)
	if !ok || base <= 0 {
		return 0, "", false
	}

	best := -1.0
	bestGUC := ""
	for _, g := range costMarginGUCs {
		cost, fp, ok := explainCostAndShape(ctx, db, q, bindings, []string{"SET " + g + "=off"})
		if !ok {
			continue // GUC unknown on this version, or the query cannot be planned without it
		}
		// same shape means the perturbation didn't force a different plan
		if fp == baseFP {
			continue
		}
		if best < 0 || cost < best {
			best, bestGUC = cost, g
		}
	}
	if best < 0 {
		// no alternative plan exists, so the choice is forced, never a tiebreak
		return 100, "none", true
	}
	return (best - base) / base * 100, bestGUC, true
}

// explainCostAndShape plans the query once under the given settings, in a
// rolled-back transaction with SET LOCAL, so a perturbation cannot leak.
func explainCostAndShape(ctx context.Context, db *sql.DB, q *Query, bindings map[string]any, sets []string) (float64, string, bool) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", false
	}
	defer tx.Rollback() //nolint:errcheck // read-only probe; rollback is the point

	// query's own GUCs first, then the probe's sets: the sets are the
	// perturbation being priced and must win any overlap
	applyGUCsTx(ctx, tx, q.GetGUCs())
	for _, s := range sets {
		if _, err := tx.ExecContext(ctx, strings.Replace(s, "SET ", "SET LOCAL ", 1)); err != nil {
			return 0, "", false
		}
	}
	if err := applySearchPath(ctx, tx, q.GetSearchPath(), true); err != nil {
		return 0, "", false
	}

	sqlText := q.OrdinalQuery
	var args []any
	if len(q.Args) > 0 {
		sqlText, args = q.Prepare(bindings)
	}
	ex, err := ExecuteExplain(ctx, tx, sqlText, args...)
	if err != nil {
		return 0, "", false
	}
	var sb strings.Builder
	planFingerprint(&ex.Plan, &sb)
	return ex.Plan.TotalCost, sb.String(), true
}
