package regresql

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Querier is an interface that both *sql.DB and *sql.Tx implement
type Querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
}

// OpenDB opens a database connection
func OpenDB(pguri string) (*sql.DB, error) {
	return sql.Open("pgx", pguri)
}
