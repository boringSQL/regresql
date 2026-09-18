package regresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// SQLSTATE for statement_timeout cancellation.
const pgQueryCanceled = "57014"

// resolveTimeout: per-query metadata override wins over the global default (0 = none).
func resolveTimeout(q *Query) time.Duration {
	if opts := q.GetRegressQLOptions(); opts.Timeout > 0 {
		return opts.Timeout
	}
	return GetStatementTimeout()
}

func applyStatementTimeout(ctx context.Context, q Querier, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	stmt := fmt.Sprintf("SET LOCAL statement_timeout = %d", d.Milliseconds())
	if _, err := q.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("failed to set statement_timeout: %w", err)
	}
	return nil
}

// applySearchPath sets the per-query search_path, SET LOCAL inside a
// transaction so it cannot leak into the next query on a pooled connection.
// execer is narrower than Querier on purpose: the admit path holds a *sql.Conn.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func applySearchPath(ctx context.Context, q execer, sp string, local bool) error {
	if sp == "" {
		return nil
	}
	if !searchPathRx.MatchString(sp) {
		return fmt.Errorf("refusing malformed search_path %q", sp)
	}
	kw := "SET"
	if local {
		kw = "SET LOCAL"
	}
	if _, err := q.ExecContext(ctx, kw+" search_path = "+sp); err != nil {
		return fmt.Errorf("failed to set search_path: %w", err)
	}
	return nil
}

// identifiers, commas and spaces only
var searchPathRx = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*(\s*,\s*[A-Za-z_][A-Za-z0-9_$]*)*$`)

func isTimeoutError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgQueryCanceled
}
