package regresql

import (
	"database/sql"

	_ "github.com/lib/pq"
)

// OpenDB opens a database connection
func OpenDB(pguri string) (*sql.DB, error) {
	return sql.Open("postgres", pguri)
}
