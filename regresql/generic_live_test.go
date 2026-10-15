package regresql

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func liveGenericDB(t *testing.T) *sql.DB {
	t.Helper()
	uri := os.Getenv("REGRESQL_TEST_PGURI")
	if uri == "" {
		t.Skip("REGRESQL_TEST_PGURI not set")
	}
	plain, err := OpenDB(uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { plain.Close() })
	t.Cleanup(func() { _, _ = plain.Exec(`DROP TABLE IF EXISTS gen_t`) })
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS gen_t`,
		`CREATE TABLE gen_t(id int primary key, grp int, v text)`,
		`INSERT INTO gen_t SELECT i, i % 10, 'row' || i FROM generate_series(1, 2000) i`,
		`ANALYZE gen_t`,
	} {
		if _, err := plain.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	setInjected(t, true)
	db, err := openSessionDB(uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func liveQuery(t *testing.T, header, body string) *Query {
	t.Helper()
	path := filepath.Join(t.TempDir(), "q.sql")
	if err := os.WriteFile(path, []byte("-- name: q\n"+header+body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	qs, err := parseQueryFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return qs["q"]
}

func ids(vals ...any) []map[string]any {
	out := make([]map[string]any, len(vals))
	for i, v := range vals {
		out[i] = map[string]any{"id": v}
	}
	return out
}

func TestLiveGenericRunsNothing(t *testing.T) {
	db := liveGenericDB(t)
	ctx := context.Background()

	counters := func() [4]int64 {
		t.Helper()
		var c [4]int64
		if _, err := db.ExecContext(ctx, `SELECT pg_stat_force_next_flush()`); err != nil {
			t.Fatal(err)
		}
		err := db.QueryRowContext(ctx, `SELECT seq_scan, coalesce(idx_scan, 0), n_tup_upd, n_tup_del FROM pg_stat_user_tables WHERE relname = 'gen_t'`).
			Scan(&c[0], &c[1], &c[2], &c[3])
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	for name, body := range map[string]string{
		"select": `select * from gen_t where id = :id`,
		"update": `update gen_t set v = 'changed' where id = :id`,
		"delete": `delete from gen_t where id = :id`,
	} {
		t.Run(name, func(t *testing.T) {
			q := liveQuery(t, "", body)
			before := counters()

			out, err := checkGeneric(ctx, db, q, []string{"1", "2", "3"}, ids(1, 2, 3))
			if err != nil {
				t.Fatal(err)
			}
			if out.Skipped || out.Failed {
				t.Fatalf("unexpected outcome %+v", out)
			}
			if after := counters(); after != before {
				t.Errorf("table was touched: %v -> %v", before, after)
			}

			var rows int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gen_t WHERE v = 'changed'`).Scan(&rows); err != nil || rows != 0 {
				t.Errorf("changed rows = %d, %v", rows, err)
			}
		})
	}
}

func TestLiveGenericParamTypes(t *testing.T) {
	db := liveGenericDB(t)
	ctx := context.Background()
	names := []string{"1"}
	bindings := []map[string]any{{"a": 1, "b": 2}}
	body := `select * from gen_t where id = :a + :b`

	out, err := checkGeneric(ctx, db, liveQuery(t, "", body), names, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Skipped || !strings.Contains(out.Message, "not unique") {
		t.Errorf("untyped: %+v", out)
	}

	out, err = checkGeneric(ctx, db, liveQuery(t, "-- paramtypes: int, int\n", body), names, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if out.Skipped || out.Failed {
		t.Errorf("typed: %+v", out)
	}

	out, err = checkGeneric(ctx, db, liveQuery(t, "-- paramtypes: int\n", body), names, bindings)
	if err != nil || !out.Skipped || !strings.Contains(out.Message, "paramtypes") {
		t.Errorf("count mismatch: %+v, %v", out, err)
	}
}

func TestLiveGenericCleansUp(t *testing.T) {
	db := liveGenericDB(t)
	db.SetMaxOpenConns(1)
	ctx := context.Background()

	leftover := func() int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_prepared_statements WHERE name = 'regresql_generic'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("server error mid run", func(t *testing.T) {
		q := liveQuery(t, "-- paramtypes: int\n", `select * from gen_t where id = :id`)
		out, err := checkGeneric(ctx, db, q, []string{"1", "2"}, ids(1, "abc"))
		if err != nil || !out.Skipped {
			t.Fatalf("outcome %+v, %v", out, err)
		}
		if n := leftover(); n != 0 {
			t.Errorf("%d prepared statements left", n)
		}
	})

	t.Run("timeout after prepare", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `CREATE OR REPLACE FUNCTION gen_slow(i int) RETURNS int IMMUTABLE LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.5); RETURN i; END $$`); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DROP FUNCTION IF EXISTS gen_slow(int)`) })

		q := liveQuery(t, "-- regresql: timeout:100ms\n", `select * from gen_t where id = gen_slow(:id)`)
		out, err := checkGeneric(ctx, db, q, []string{"1"}, ids(1))
		if err != nil {
			t.Fatal(err)
		}
		if !out.Failed || !strings.Contains(out.Message, "timeout") {
			t.Errorf("outcome %+v", out)
		}
		if n := leftover(); n != 0 {
			t.Errorf("%d prepared statements left", n)
		}
	})
}

func TestLiveGenericOtherErrorsSurface(t *testing.T) {
	db := liveGenericDB(t)
	q := liveQuery(t, "", `select * from no_such_table where id = :id`)
	_, err := checkGeneric(context.Background(), db, q, []string{"1"}, ids(1))
	if err == nil {
		t.Fatal("missing table must surface as an error")
	}
}

func TestLiveGenericNoParameters(t *testing.T) {
	db := liveGenericDB(t)
	q := liveQuery(t, "", `select * from gen_t`)
	out, err := checkGeneric(context.Background(), db, q, []string{"1"}, ids(1))
	if err != nil || !out.Skipped {
		t.Errorf("outcome %+v, %v", out, err)
	}
}

func TestLiveGenericNeedsExtension(t *testing.T) {
	uri := os.Getenv("REGRESQL_TEST_PGURI")
	if uri == "" {
		t.Skip("REGRESQL_TEST_PGURI not set")
	}
	setInjected(t, false)
	db, err := OpenDB(uri)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	q := liveQuery(t, "", `select * from pg_class where oid = :id`)
	_, err = checkGeneric(context.Background(), db, q, []string{"1"}, ids(1))
	if !errors.Is(err, errGenericNoExtension) {
		t.Errorf("err = %v", err)
	}
}
