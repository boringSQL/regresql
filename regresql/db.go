package regresql

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// OpenDB opens a database connection
func OpenDB(pguri string) (*sql.DB, error) {
	return sql.Open("pgx", pguri)
}
