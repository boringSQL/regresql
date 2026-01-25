package regresql

import (
	"database/sql"
	"fmt"
	"os"
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
	if _, err := db.Exec(string(content)); err != nil {
		return fmt.Errorf("applying stats from %s: %w", file, err)
	}

	return nil
}
