package regresql

import (
	"context"
	"errors"
	"fmt"
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

func isTimeoutError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgQueryCanceled
}
