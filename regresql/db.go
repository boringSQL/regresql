package regresql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// injectedStats means a stats file is in use, so every planning session must
// have pg_regresql loaded.
var injectedStats bool

// Querier is an interface that both *sql.DB and *sql.Tx implement
type Querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// OpenDB opens a database connection
func OpenDB(pguri string) (*sql.DB, error) {
	return sql.Open("pgx", pguri)
}

// openSessionDB opens a pool that LOADs pg_regresql on every connection when
// stats are injected, so a missing extension fails instead of yielding a plan.
func openSessionDB(pguri string) (*sql.DB, error) {
	if !injectedStats {
		return OpenDB(pguri)
	}
	cfg, err := pgx.ParseConfig(pguri)
	if err != nil {
		return nil, err
	}
	load := func(ctx context.Context, c *pgx.Conn) error {
		if _, err := c.Exec(ctx, "LOAD 'pg_regresql'"); err != nil {
			_ = c.Close(ctx)
			return fmt.Errorf("loading pg_regresql (install it into $libdir, or $libdir/plugins for non-superusers; see pg_ext/): %w", err)
		}
		return nil
	}
	return sql.OpenDB(stdlib.GetConnector(*cfg, stdlib.OptionAfterConnect(load))), nil
}
