# RegreSQL

Regression testing for SQL queries. Write queries, capture expected results, compare when their cost or I/O characteristics change, detect when something changes.

RegreSQL finds your `*.sql` files, runs them against PostgreSQL, and compares output to known-good baselines. When a query's result changes unexpectedly, you'll know immediately.

## Installing

### Homebrew (macOS)

```bash
brew tap boringsql/boringsql
brew install regresql
```

### Go

```bash
go install github.com/boringsql/regresql@latest
```

Binary goes to `$GOPATH/bin` (defaults to `~/go/bin`).

### Requirements

Snapshot commands need PostgreSQL client tools (`pg_dump`, `pg_restore`, `psql`):

```bash
# macOS
brew install libpq

# Debian/Ubuntu
apt install postgresql-client

# RHEL/Fedora
dnf install postgresql
```

## Quick Start

```bash
# Initialize in your project
regresql init postgres://localhost/mydb

# See what SQL files exist
regresql discover

# Add queries to the test suite
regresql add src/sql/

# Edit plan files to set parameter values (if your queries have parameters)
vim regresql/plans/src/sql/users.yaml

# Capture expected output
regresql update

# Run tests
regresql test
```

## Core Commands

### `regresql discover`

Shows all SQL files and their test status:

```
$ regresql discover
[+] src/sql/users.sql           # all queries have plans
[ ] src/sql/orders.sql          # no plans yet
[~] src/sql/products.sql        # partial coverage
```

Use `--queries` to see individual query status within files.

### `regresql add <path...>`

Adds SQL files to your test suite by creating plan files:

```bash
regresql add src/sql/users.sql      # single file
regresql add src/sql/                # entire directory
regresql add "src/**/*.sql"          # glob pattern
```

### `regresql remove <path...>`

Removes files from the test suite:

```bash
regresql remove src/sql/old_query.sql
regresql remove src/sql/ --clean     # also delete expected/baseline files
regresql remove src/sql/ --dry-run   # preview what would be deleted
```

### `regresql update`

Captures current query output as the expected baseline:

```bash
regresql update                      # all queries
regresql update src/sql/users.sql    # specific file
regresql update --pending            # only queries without expected files
regresql update --interactive        # review each change
```

### `regresql test`

Runs queries and compares output against expected results:

```bash
regresql test
regresql test --run "user"           # filter by regexp
regresql test --format junit -o results.xml
```

Output formats: `console` (default), `pgtap`, `junit`, `json`, `github-actions`

### `regresql baseline`

Creates EXPLAIN cost baselines to detect query plan regressions:

```bash
regresql baseline
regresql baseline --analyze          # include actual timing
```

## SQL Query Files

RegreSQL works with standard SQL files. Multiple queries per file are supported with `-- name:` annotations:

```sql
-- name: get-user-by-id
SELECT * FROM users WHERE id = :id;

-- name: list-active-users
SELECT * FROM users WHERE active = true;
```

Single-query files don't need annotations—the filename becomes the query name.

### Query Parameters

Named (`:param`) and positional (`$1`) parameters are supported. Set values in plan files:

```yaml
# regresql/plans/src/sql/users.yaml
"1":
  id: 42
"2":
  id: 100
```

Each numbered entry runs as a separate test case.

### Query Metadata

Control test behavior per-query:

```sql
-- name: expensive-report
-- regresql: nobaseline, noseqscanwarn
SELECT ...
```

Options: `notest`, `nobaseline`, `noseqscanwarn`, `difffloattolerance:0.01`

## Snapshots

Snapshots capture database state for reproducible tests. Build once, restore before each test run.

```yaml
# regresql/regress.yaml
pguri: postgres://localhost/mydb
snapshot:
  schema: db/schema.sql
  migrations: db/migrations/
```

```bash
regresql snapshot build      # create snapshot
regresql snapshot restore    # restore to database
regresql snapshot info       # view metadata
regresql test                # auto-restores before testing
```

Snapshots track hashes of schema and migrations. If sources change, `regresql test` fails with instructions to rebuild.

### Snapshot Versioning

Tag snapshots for comparison across versions:

```bash
regresql snapshot tag v1.0
regresql snapshot tag post-migration --note "After user table refactor"
regresql snapshot list
regresql diff --from v1.0 --to current
```

## Fixturize

RegreSQL is fully integrated with [fixturize](https://github.com/boringSQL/fixturize), providing ability to capture consistent data sub-graphs from a PostgreSQL database and apply them for snapshot building.

For more help check `fixturize` repository or try `regresql fixturize`.

## Migration Testing

Test how migrations affect query output:

```bash
regresql migrate --script db/migrations/001_add_column.sql
regresql migrate --command "goose up"
```

Runs all queries before and after the migration, reports differences.

## Ignoring Files

Create `.regresignore` (gitignore syntax):

```
*_test.sql
db/migrations/
```

Or in config:

```yaml
# regresql/regress.yaml
ignore:
  - "*_test.sql"
  - "db/migrations/"
```

## Configuration

```yaml
# regresql/regress.yaml
pguri: postgres://localhost/mydb
root: "."

plan_quality:
  ignore_seqscan_tables:
    - genre
    - media_type

snapshot:
  schema: db/schema.sql
  migrations: db/migrations/
  fixtures: [users, products]
```

## File Structure

```
regresql/
├── regress.yaml           # configuration
├── plans/                 # parameter bindings
│   └── src/sql/
│       └── users.yaml
├── expected/              # expected query output
│   └── src/sql/
│       └── users.1.json
├── baselines/             # EXPLAIN cost baselines
│   └── src/sql/
│       └── users.1.json
└── out/                   # test run output (for comparison)
```

## History

Fork of the original [regresql](https://github.com/dimitri) by Dimitri Fontaine, from [Mastering PostgreSQL](http://masteringpostgresql.com/). Extended as part of the [boringSQL](https://boringsql.com) project.

## License

[BSD-2-Clause](LICENSE)
