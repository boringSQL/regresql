package regresql

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseQueryString(t *testing.T) {
	queryString := `select * from foo where id = :user_id`
	q, err := NewQueryFromString("default", queryString)
	if err != nil {
		t.Fatalf("NewQueryFromString failed: %v", err)
	}

	if len(q.NamedArgs) != 1 || q.NamedArgs[0].Name != "user_id" {
		t.Error("Expected unique arg [\"user_id\"], got ", q.NamedArgs)
	}
	if len(q.Args) != 1 || q.Args[0] != "user_id" {
		t.Error("Expected args [\"user_id\"], got ", q.Args)
	}
}

func TestParseQueryStringWithTypeCast(t *testing.T) {
	queryString := `select name::text from foo where id = :user_id`
	q, err := NewQueryFromString("default", queryString)
	if err != nil {
		t.Fatalf("NewQueryFromString failed: %v", err)
	}

	if len(q.NamedArgs) != 1 || q.NamedArgs[0].Name != "user_id" {
		t.Error("Expected unique arg [\"user_id\"], got ", q.NamedArgs)
	}
	if len(q.Args) != 1 || q.Args[0] != "user_id" {
		t.Error("Expected args [\"user_id\"], got ", q.Args)
	}
}

func TestPrepareOneParam(t *testing.T) {
	queryString := `select * from foo where id = :id`
	q, err := NewQueryFromString("default", queryString)
	if err != nil {
		t.Fatalf("NewQueryFromString failed: %v", err)
	}
	b := make(map[string]any)
	b["id"] = "1"

	sql, params := q.Prepare(b)

	expected := "-- name: default\nselect * from foo where id = $1"
	if sql != expected {
		t.Errorf("Query string not as expected.\nGot:\n%s\n\nExpected:\n%s", sql, expected)
	}

	if !(len(params) == 1 &&
		params[0] == "1") {
		t.Error("Bindings not properly applied, got ", params)
	}
}

func TestPrepareTwoParams(t *testing.T) {
	queryString := `select * from foo where a = :a and b between :a and :b`
	q, err := NewQueryFromString("default", queryString)
	if err != nil {
		t.Fatalf("NewQueryFromString failed: %v", err)
	}
	b := make(map[string]any)
	b["a"] = "a"
	b["b"] = "b"

	sql, params := q.Prepare(b)

	expected := "-- name: default\nselect * from foo where a = $1 and b between $1 and $2"
	if sql != expected {
		t.Errorf("Query string not as expected.\nGot:\n%s\n\nExpected:\n%s", sql, expected)
	}

	// Prepare deduplicates parameters, so :a only appears once
	if !(len(params) == 2 &&
		params[0] == "a" &&
		params[1] == "b") {
		t.Error("Bindings not properly applied, got ", params)
	}
}

func TestGenericMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.sql")
	content := `-- name: typed
-- regresql: NoGeneric
-- paramtypes: timestamptz, numeric(10,2)
select * from t where ts > :since and n = :n and ts < :since + 1;

-- name: wrongcount
-- paramtypes: int
select * from t where a = :a and b = :b;

-- name: plain
select * from t where a = :a;
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	qs, err := parseQueryFile(path)
	if err != nil {
		t.Fatal(err)
	}

	typed := qs["typed"]
	if !typed.GetRegressQLOptions().NoGeneric {
		t.Error("NoGeneric not set")
	}
	types, err := typed.GetParamTypes()
	if err != nil || len(types) != 2 || types[0] != "timestamptz" || types[1] != "numeric(10,2)" {
		t.Errorf("GetParamTypes = %v, %v", types, err)
	}

	if _, err := qs["wrongcount"].GetParamTypes(); err == nil {
		t.Error("count mismatch not reported")
	}

	plain := qs["plain"]
	if plain.GetRegressQLOptions().NoGeneric {
		t.Error("NoGeneric set on plain query")
	}
	if types, err := plain.GetParamTypes(); types != nil || err != nil {
		t.Errorf("plain GetParamTypes = %v, %v", types, err)
	}
}
