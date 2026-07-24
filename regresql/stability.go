package regresql

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
)

const (
	// DefaultStabilityReps is the MINIMUM number of re-ANALYZE rounds
	DefaultStabilityReps = 3

	// stabilityQuietRounds: rounds with no new tie before the set is called final.
	stabilityQuietRounds = 3
	// stabilityMaxRounds limits the number of loop iterations if ties continue to happen
	stabilityMaxRounds = 30
)

func bindingKey(queryName, bindingName string) string {
	return queryName + "\x00" + bindingName
}

// stabilityPass re-ANALYZEs base and re-plans each query; a plan that isn't
// constant is a cost tie. Runs `reps` rounds, then until the tie set stops
// growing, so the exclusions are a fixed point, not a function of reps.
// Mutates base stats.
func stabilityPass(ctx context.Context, db *sql.DB, pqs []*PlannedQuery, suite *Suite, reps int) map[string]string {
	first := map[string]string{}
	unstable := map[string]string{}
	quiet := 0

	for r := 0; r < stabilityMaxRounds && (r < reps || quiet < stabilityQuietRounds); r++ {
		found := len(unstable)
		if _, err := db.ExecContext(ctx, "ANALYZE"); err != nil {
			return unstable // can't resample; leave everything untested
		}
		for _, pq := range pqs {
			if !suite.matchesRunFilter(filepath.Base(pq.SQLPath), pq.Query.Name) {
				continue
			}
			if pq.Query.GetRegressQLOptions().NoTest {
				continue
			}
			for _, b := range iterateBindings(pq.Plan) {
				key := bindingKey(pq.Query.Name, b.name)
				if _, done := unstable[key]; done {
					continue
				}
				fp := planFingerprintOf(ctx, db, pq.Query, b.bindings)
				if fp == "" {
					continue
				}
				if f, ok := first[key]; !ok {
					first[key] = fp
				} else if f != fp {
					unstable[key] = "plan unstable across re-ANALYZE (cost tie)"
				}
			}
		}
		if len(unstable) == found {
			quiet++
		} else {
			quiet = 0
		}
	}
	return unstable
}

// planFingerprintOf returns an order-sensitive tree fingerprint, so a re-ordered
// join (same nodes, different order) counts as a change, not just shape flips.
func planFingerprintOf(ctx context.Context, db *sql.DB, q *Query, bindings map[string]any) string {
	sqlText := q.OrdinalQuery
	var args []any
	if len(q.Args) > 0 {
		sqlText, args = q.Prepare(bindings)
	}
	ex, err := ExecuteExplain(ctx, db, sqlText, args...)
	if err != nil {
		return ""
	}
	var b strings.Builder
	planFingerprint(&ex.Plan, &b)
	return b.String()
}

// planFingerprint serializes node type + relation + children recursively.
func planFingerprint(n *PlanNode, b *strings.Builder) {
	b.WriteString(n.NodeType)
	if n.RelationName != "" {
		b.WriteByte('(')
		b.WriteString(n.RelationName)
		b.WriteByte(')')
	}
	if len(n.Plans) > 0 {
		b.WriteByte('[')
		for i := range n.Plans {
			if i > 0 {
				b.WriteByte(',')
			}
			planFingerprint(&n.Plans[i], b)
		}
		b.WriteByte(']')
	}
}
