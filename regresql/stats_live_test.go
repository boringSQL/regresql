package regresql

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
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

func TestLiveCollectEnv(t *testing.T) {
	uri := os.Getenv("REGRESQL_TEST_PGURI")
	if uri == "" {
		t.Skip("REGRESQL_TEST_PGURI not set")
	}
	plain, err := OpenDB(uri)
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	_, loadErr := plain.Exec("LOAD 'pg_regresql'")

	setInjected(t, true)
	env := CollectEnv(uri, "", config{PgUri: uri}, t.TempDir(), false)
	if !env.Connected {
		t.Fatalf("not connected: %v", env.Detail)
	}
	if (loadErr == nil) != (env.PgRegresql == envLoaded) {
		t.Errorf("probe %q disagrees with LOAD result %v", env.PgRegresql, loadErr)
	}
	if env.Failed() != (loadErr != nil) {
		t.Errorf("Failed() = %v with LOAD err %v", env.Failed(), loadErr)
	}

	injectedStats = false
	if CollectEnv(uri, "", config{PgUri: uri}, t.TempDir(), false).Failed() {
		t.Error("buffers mode must not fail without the extension")
	}
}

func TestLiveApplyStatistics(t *testing.T) {
	uri := os.Getenv("REGRESQL_TEST_PGURI")
	if uri == "" {
		t.Skip("REGRESQL_TEST_PGURI not set")
	}
	db, err := OpenDB(uri)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _, _ = db.Exec(`DROP TABLE IF EXISTS apply_t`) })
	for _, q := range []string{`DROP TABLE IF EXISTS apply_t`, `CREATE TABLE apply_t(id int primary key)`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	file := filepath.Join(t.TempDir(), "stats.sql")
	content := "--\n-- Dumped from database version 18.6\n--\n\\restrict tok\n" +
		"SELECT set_config('search_path', '', false);\n" +
		"SELECT * FROM pg_catalog.pg_restore_relation_stats('version', '180006'::integer, 'schemaname', 'public', 'relname', 'apply_t', 'relpages', '500'::integer, 'reltuples', '123456'::real, 'relallvisible', '0'::integer);\n" +
		"\\unrestrict tok\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var before, after string
	if err := db.QueryRow(`SHOW search_path`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := ApplyStatistics(db, file); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SHOW search_path`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("search_path leaked into the pool: %q -> %q", before, after)
	}
	var tuples float64
	if err := db.QueryRow(`SELECT reltuples FROM pg_class WHERE oid = 'public.apply_t'::regclass`).Scan(&tuples); err != nil {
		t.Fatal(err)
	}
	if tuples != 123456 {
		t.Errorf("reltuples = %v, want 123456", tuples)
	}
}
