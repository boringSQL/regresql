package regresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func flipFixture(t *testing.T, db *sql.DB) *Query {
	t.Helper()
	t.Cleanup(func() { _, _ = db.Exec(`DROP TABLE IF EXISTS gen_big`) })
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS gen_big`,
		`CREATE TABLE gen_big(id int primary key, status int, payload text)`,
		`CREATE INDEX gen_big_status ON gen_big(status)`,
		`SELECT pg_restore_relation_stats('schemaname','public','relname','gen_big','relpages',100000::int,'reltuples',10000000::real,'relallvisible',100000::int)`,
		`SELECT pg_restore_relation_stats('schemaname','public','relname','gen_big_status','relpages',27000::int,'reltuples',10000000::real)`,
		`SELECT pg_restore_attribute_stats('schemaname','public','relname','gen_big','attname','status','inherited',false,'null_frac',0::real,'avg_width',4,'n_distinct',10::real,'correlation',0::real,'most_common_vals','{1,2,3}'::text,'most_common_freqs','{0.5,0.3,0.00001}'::real[])`,
		`SELECT pg_restore_attribute_stats('schemaname','public','relname','gen_big','attname','payload','inherited',false,'null_frac',0::real,'avg_width',20,'n_distinct',-1::real)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	for status, wantIndexed := range map[int]bool{1: false, 2: false, 3: true} {
		var plan string
		rows, err := db.Query(fmt.Sprintf(`EXPLAIN select * from gen_big where status = %d order by payload`, status))
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan += line + "\n"
		}
		rows.Close()
		if indexed := strings.Contains(plan, "Bitmap Heap Scan") || strings.Contains(plan, "Index Scan"); indexed != wantIndexed {
			t.Fatalf("fixture drifted: status %d, indexed=%v, want %v\n%s", status, indexed, wantIndexed, plan)
		}
	}
	return liveQuery(t, "", `select * from gen_big where status = :s order by payload`)
}

func TestLiveGenericFlip(t *testing.T) {
	db := liveGenericDB(t)
	var version int
	if err := db.QueryRow(`SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < MinPGVersionForStats {
		t.Skip("pg_restore_attribute_stats needs PostgreSQL 18")
	}
	q := flipFixture(t, db)
	ctx := context.Background()

	bind := func(vals ...int) []map[string]any {
		out := make([]map[string]any, len(vals))
		for i, v := range vals {
			out[i] = map[string]any{"s": v}
		}
		return out
	}

	tests := []struct {
		name     string
		names    []string
		bindings []map[string]any
		failed   bool
		message  string
	}{
		{
			"broad first, selective last settles and flips",
			[]string{"1_broad", "2_broad", "3_broad", "4_broad", "5_broad", "6_selective"},
			bind(1, 2, 1, 2, 1, 3),
			true,
			`generic: gen_big index scan -> Seq Scan; binding "6_selective"`,
		},
		{
			"only selective bindings do not settle",
			[]string{"1", "2", "3"},
			bind(3, 3, 3),
			false,
			"generic plan would Seq Scan gen_big; settles if broad bindings dominate",
		},
		{
			"only broad bindings settle without a flip",
			[]string{"1", "2", "3"},
			bind(1, 2, 1),
			false,
			"settled; no index->seq flip",
		},
		{
			"four selective among the first five do not settle",
			[]string{"1", "2", "3", "4", "5", "6"},
			bind(3, 3, 3, 3, 1, 3),
			false,
			"generic plan would Seq Scan gen_big; settles if broad bindings dominate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := checkGeneric(ctx, db, q, tt.names, tt.bindings)
			if err != nil {
				t.Fatal(err)
			}
			if out.Skipped || out.Failed != tt.failed || out.Message != tt.message {
				t.Errorf("outcome %+v, want failed=%v %q", out, tt.failed, tt.message)
			}
		})
	}
}
