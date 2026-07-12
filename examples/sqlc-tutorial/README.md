# sqlc + RegreSQL

This is the [sqlc getting-started project](https://docs.sqlc.dev/en/latest/tutorials/getting-started-postgresql.html)
(an `authors` table and five queries) with RegreSQL added on top.

sqlc reads `query.sql` and `schema.sql` and generates type-safe Go. In doing so
it verifies every query *compiles* against the schema: the columns and types line
up. What it doesn't check is behavior. A query that still compiles can start
returning different rows after someone edits it or runs a migration, and sqlc
regenerates cleanly and says nothing. RegreSQL tests that part: it runs the
queries and compares their output, and their query plan, against a committed
baseline.

## What's here

```
schema.sql              the authors table
query.sql               the sqlc queries (RegreSQL reads this file directly)
sqlc.yaml               sqlc config
tutorial/               Go that sqlc generated from the two files above
db/seed.sql             a few authors, loaded into the snapshot
regresql/
  regress.yaml          connection + snapshot config
  plans/                parameter values per query
  expected/             committed query output
  baselines/            committed EXPLAIN baselines
snapshots/default.dump  the seeded database, built from schema.sql + db/seed.sql
```

RegreSQL reads `query.sql` as-is. It understands sqlc's `-- name: GetAuthor :one`
annotations (the `:one`/`:many`/`:exec` suffix and all), so there's no second
copy of the queries to keep in sync.

## Reads and writes

RegreSQL compares what a query returns, so it tests the read queries and leaves
the writes alone. The three mutating queries carry a metadata line:

```sql
-- name: CreateAuthor :one
-- regresql: notest
INSERT INTO authors ...
```

`notest` keeps `CreateAuthor`, `UpdateAuthor`, and `DeleteAuthor` out of the run.
`GetAuthor` and `ListAuthors` are the two that get tested.

## Run it

Start a database:

```bash
docker run -d --name sqlc-authors \
  -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app -e POSTGRES_DB=sqlc_test \
  -p 5432:5432 postgres:18
```

`regresql/regress.yaml` points at `postgres://localhost/sqlc_test`; set
`DATABASE_URL` if yours is elsewhere. Then restore the seeded snapshot and run
the tests:

```bash
regresql snapshot restore
regresql test
```

```
Running regression tests...

RESULTS:
  ✓ 4 passing
```

Two queries, each checked for output and for plan cost.

## Catch a change

Say you "clean up" `ListAuthors` to only return authors who have a bio:

```sql
-- name: ListAuthors :many
SELECT * FROM authors
WHERE bio IS NOT NULL
ORDER BY name;
```

sqlc regenerates without complaint; the query is valid against the schema. But
it now returns fewer rows, and `regresql test` says so:

```
FAILING:
  query_ListAuthors.json
  COMPARISON SUMMARY:
  ├─ Expected: 3 rows
  ├─ Actual:   2 rows
  └─ Result:   1 rows removed
  REMOVED ROWS (showing 1 of 1):
  {id: 3, name: "Octavia E. Butler", bio: null}
```

The change compiled and dropped a row. If it was a mistake, you caught it before
merge. If it was intended, `regresql update query.sql` accepts the new output and
you commit it alongside the query.

## Next

- The full RegreSQL loop from scratch: the [getting-started guide](../../docs/getting-started.md).
- Parameters, plan baselines, CI, snapshots: the [main README](../../README.md).
