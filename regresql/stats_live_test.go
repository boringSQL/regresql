package regresql

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
)

// Needs PG 18 with pg_regresql installed; set REGRESQL_TEST_PGURI to run.
func TestLiveInjection(t *testing.T) {
	uri := os.Getenv("REGRESQL_TEST_PGURI")
	if uri == "" {
		t.Skip("REGRESQL_TEST_PGURI not set")
	}
	plain, _ := OpenDB(uri)
	defer plain.Close()
	t.Cleanup(func() { _, _ = plain.Exec(`DROP TABLE IF EXISTS inj_t`) })
	for _, q := range []string{
		`DROP TABLE IF EXISTS inj_t`,
		`CREATE TABLE inj_t(id int primary key, v text)`,
		`SELECT pg_restore_relation_stats('schemaname','public','relname','inj_t','relpages',10000::int,'reltuples',1000000::real,'relallvisible',10000::int)`,
	} {
		if _, err := plain.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	// without the hook: tripwire must fire
	injectedStats = false
	if err := VerifyInjection(plain); err == nil {
		t.Fatal("expected tripwire error without pg_regresql loaded")
	} else {
		t.Logf("no-hook: %s", strings.TrimSpace(err.Error()))
	}

	// with the hook: every pooled conn loaded, tripwire passes
	injectedStats = true
	defer func() { injectedStats = false }()
	db, err := openSessionDB(uri)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(3)
	var conns []*sql.Conn
	for i := 0; i < 3; i++ {
		c, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
		var v string
		if err := c.QueryRowContext(context.Background(), `SHOW pg_regresql.active`).Scan(&v); err != nil {
			t.Fatalf("conn %d: %v", i, err)
		}
	}
	for _, c := range conns {
		c.Close()
	}
	if err := VerifyInjection(db); err != nil {
		t.Fatalf("with hook: %v", err)
	}
}
