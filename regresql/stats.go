package regresql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
)

// For PostgreSQL 18+ checks
const MinPGVersionForStats = 180000

// ApplyStatistics applies an external statistics file to the database.
// Requires PostgreSQL 18+ for pg_restore_relation_stats/pg_restore_attribute_stats.
func ApplyStatistics(db *sql.DB, file string) error {
	serverCtx, err := CaptureServerContext(db)
	if err != nil {
		return fmt.Errorf("failed to get server version: %w", err)
	}

	if serverCtx.VersionNum < MinPGVersionForStats {
		return fmt.Errorf("--stats requires PostgreSQL 18+ (found %s, version_num=%d)",
			serverCtx.Version, serverCtx.VersionNum)
	}

	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("reading stats file %s: %w", file, err)
	}
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("applying stats from %s: %w", file, err)
	}
	defer conn.Close()

	_, applyErr := conn.ExecContext(ctx, stripPsqlMeta(string(content)))
	// do not leak pg_dump's search_path into the pool
	if _, err := conn.ExecContext(ctx, "RESET search_path"); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	if applyErr != nil {
		return fmt.Errorf("applying stats from %s: %w", file, applyErr)
	}

	return nil
}

var psqlRestrictRe = regexp.MustCompile(`(?m)^\\(un)?restrict([ \t][^\n]*)?\r?(\n|$)`)

// stripPsqlMeta drops pg_dump's \restrict / \unrestrict lines
func stripPsqlMeta(sql string) string {
	return psqlRestrictRe.ReplaceAllString(sql, "")
}

type divergentRelation struct {
	rel                string
	reltuples          float64
	relpages, physical int64
}

// mostDivergent returns the relation whose recorded size differs most from its
// physical size, or nil when none diverges.
func mostDivergent(db *sql.DB) (*divergentRelation, error) {
	var d divergentRelation
	err := db.QueryRow(`
SELECT rel, reltuples, relpages, phys FROM (
  SELECT c.oid::regclass::text AS rel, c.reltuples::float8 AS reltuples, c.relpages::bigint AS relpages,
         pg_relation_size(c.oid) / current_setting('block_size')::bigint AS phys
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE c.relkind = 'r' AND NOT c.relhassubclass AND NOT c.relispartition AND NOT c.relrowsecurity
    AND n.nspname NOT IN ('pg_catalog', 'information_schema')
    AND n.nspname NOT LIKE 'pg\_toast%' AND n.nspname NOT LIKE 'pg\_temp%'
    AND c.relpages > 0 AND c.reltuples >= 0 AND has_table_privilege(c.oid, 'SELECT')
) t
WHERE relpages > 2 * phys OR phys > 2 * relpages
ORDER BY abs(ln((relpages + 1.0) / (phys + 1.0))) DESC
LIMIT 1`).Scan(&d.rel, &d.reltuples, &d.relpages, &d.physical)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// StatsPresent reports whether the database already carries injected stats.
func StatsPresent(pguri string) (bool, error) {
	db, err := OpenDB(pguri)
	if err != nil {
		return false, err
	}
	defer db.Close()
	d, err := mostDivergent(db)
	return d != nil, err
}

// VerifyInjection fails when the planner ignores injected stats, by EXPLAINing
// the relation whose injected size differs most from its physical size.
func VerifyInjection(db *sql.DB) error {
	d, err := mostDivergent(db)
	if err != nil {
		return fmt.Errorf("checking stats injection: %w\n", err)
	}
	if d == nil {
		return nil
	}
	rel, reltuples, relpages, physical := d.rel, d.reltuples, d.relpages, d.physical

	var raw string
	if err := db.QueryRow("EXPLAIN (FORMAT JSON) SELECT * FROM " + rel).Scan(&raw); err != nil {
		return fmt.Errorf("checking stats injection on %s: %w\n", rel, err)
	}
	var plan []struct {
		Plan struct {
			Rows float64 `json:"Plan Rows"`
		} `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(raw), &plan); err != nil || len(plan) == 0 {
		return fmt.Errorf("checking stats injection on %s: unreadable EXPLAIN output\n", rel)
	}
	if !estimateMatchesInjected(plan[0].Plan.Rows, reltuples) {
		return fmt.Errorf("injected stats are not reaching the planner: %s is estimated at %.0f rows, injected reltuples is %.0f (physical %d pages, injected %d); is pg_regresql loaded?\n",
			rel, plan[0].Plan.Rows, reltuples, physical, relpages)
	}
	return nil
}

// estimateMatchesInjected allows the planner's clamping and rounding.
func estimateMatchesInjected(est, reltuples float64) bool {
	return math.Abs(est-reltuples) <= 0.5*reltuples+1
}
