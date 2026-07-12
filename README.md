# RegreSQL

SQL queries can break silently. Schema migrations, data changes, and index modifications can alter query results and tank performance — without any test catching it.

RegreSQL is a language-agnostic SQL regression testing tool for PostgreSQL. It finds your `*.sql` files, runs them against your database, compares output to known-good baselines, and tracks EXPLAIN plan changes.  Detect broken queries and performance regressions before production.

2.0 also adds cross-version testing: running the same queries against two PostgreSQL builds and comparing how each one plans them. That's a separate workflow for planner work and version upgrades, in its own section below. If you're here to test your application's queries, the everyday path is the rest of this README.

`RegreSQL` is part of the [boringSQL](https://boringsql.com) stack alongside [qshape](https://github.com/boringSQL/qshape), [Fixturize](https://github.com/boringSQL/fixturize) and [dryrun](https://github.com/boringSQL/dryrun). See the [project page](https://boringsql.com/products/regresql/) for the full overview.

## Installing

### Homebrew (macOS)

```bash
brew tap boringsql/boringsql
brew install regresql
```

### Go

```bash
go install github.com/boringsql/regresql/v2@latest
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

`init` only writes a local `regresql/` directory and a config file. It doesn't touch your database.

`update` is the step to be deliberate about. It captures whatever your queries return right now and stores that as the expected result, so run it against a database whose contents you trust. If you don't have one lying around, [Snapshots](#snapshots) below builds a reproducible one you can restore before every run.

## Why RegreSQL

- **Language-agnostic** — works with any `.sql` file, no specific language, ORM, or database extension required
- **Snapshot testing for SQL** — expected results committed as fixtures, diff when something changes
- **Migration testing** — run queries before and after a migration, see exactly what changed
- **EXPLAIN plan baselines** — track query costs over time, detect performance regressions automatically
- **Sequential scan detection** — catch missing indexes before they hit production
- **CI/CD ready** — JUnit, GitHub Actions, pgTAP, and JSON output formats

For planner work and version upgrades (2.0):

- **Cross-version planner A/B** — run the same queries against two PostgreSQL builds and compare plans, buffers, and results (`compare --base --target`)
- **Trust filter** — inject identical statistics and skip cost-tie queries, so differences come from the planner and not from ANALYZE sampling
- **Severity policies** — re-map warning and error severities per table, and turn seq scans on critical tables into failures
- **Production-stats plan testing** — inject real statistics with `--stats` and the `pg_regresql` extension to reproduce production plans on a small local database

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
regresql test --format github-actions    # inline PR annotations
regresql test --format junit -o results.xml  # Jenkins/CI
regresql test --format pgtap            # TAP protocol
```

Output formats: `console` (default), `pgtap`, `junit`, `json`, `github-actions`

### `regresql baseline`

Tracks EXPLAIN cost estimates/I/O buffers over time. When a schema change or migration causes a query plan regression. Cost spikes, sequential scans on large tables — you'll catch it in CI before it reaches production.

In analyze mode it also checks cardinality error (q-error), disk spills, and rows processed.

```bash
regresql baseline
regresql baseline --analyze          # include actual timing
```

## Continuous integration

The point of all this is catching a broken query in a pull request instead of in production. `regresql test` exits non-zero when a result or plan check fails, so any CI runner will fail the build on it. `--format github-actions` turns each failure into an inline PR annotation.

Here's a full GitHub Actions job. It spins up a Postgres, points regresql at it, restores the snapshot, and runs the tests:

```yaml
# .github/workflows/regresql.yml
name: regresql
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    env:
      # overrides pguri from the committed regress.yaml
      DATABASE_URL: postgres://postgres:postgres@localhost/postgres
    services:
      postgres:
        image: postgres:17
        env:
          POSTGRES_PASSWORD: postgres
        ports: ["5432:5432"]
        options: >-
          --health-cmd pg_isready --health-interval 10s
          --health-timeout 5s --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
      - run: go install github.com/boringsql/regresql/v2@latest
      - run: regresql snapshot restore
      - run: regresql test --format github-actions
```

`DATABASE_URL`, when set, overrides the `pguri` in `regress.yaml` for every command. That's how you point a CI run (or a one-off local run) at a different database without editing the committed config.

The `snapshot restore` step assumes you've committed a snapshot (see below). Without one, drop that line and load your schema and data however the rest of your test suite does before `regresql test`.

## Cross-version and planner testing

These commands are for testing PostgreSQL itself or a version upgrade, not your application's queries. Skip this section if you're here for the everyday path above.

### `regresql compare --base <uri> --target <uri>`

Runs the corpus against two PostgreSQL builds and prints a scoreboard of the differences. Cost is suppressed across versions. `--stability` and `--inject-stats` filter out ANALYZE noise; `--samples` adds timing.

### `regresql admit`

Keeps only queries whose result stays the same across different plans. Ambiguous ones (like a `LIMIT` over ties) are dropped.

### `regresql metamorphic`

Turns off optimizations that should not change results and checks the rows stay the same. Finds optimizer bugs on one database, without a baseline.

### `regresql coverage --taxonomy <file>`

Reports which planner-feature cells the corpus covers and which it misses.

## Using an ORM (no .sql files)

RegreSQL tests `.sql` files, so if your queries come out of an ORM (ActiveRecord, SQLAlchemy, Prisma, Sequelize) you have nothing to point it at. [qshape](https://github.com/boringSQL/qshape) fills that gap. It reads `pg_stat_statements` from your running app, collapses the many per-ORM variants of each query into one canonical shape, and generates the RegreSQL `sql/` and `plans/` skeletons:

```bash
qshape capture "$DATABASE_URL" > clusters.json
qshape regresql-stub --in clusters.json --out .
```

You get one `.sql` file per query shape and a plan YAML with placeholder test cases to fill in. From there it's the normal loop above. Run `qshape attribute` first to have the plans filled with real sampled values instead of placeholders.

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
"1":            # test case 1
  id: 42
"2":            # test case 2
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

Options: `notest`, `nobaseline`, `noseqscanwarn`, `difffloattolerance:0.01`, `timeout:5s`

Result comparison can ignore named columns, ignore row order, tolerate float differences, and compare JSONB by value.

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

Set the `DATABASE_URL` environment variable to override `pguri` at run time — useful for CI or pointing a run at a different database without touching the committed file.

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

## Learn more

- **[boringSQL](https://boringsql.com)**, the blog and project home
- **[RegreSQL project page](https://boringsql.com/products/regresql/)**, overview and docs
- **[Regression testing for PostgreSQL queries](https://boringsql.com/posts/regresql-testing-queries/)**, the why and how
- **[RegreSQL as a PostgreSQL extension](https://boringsql.com/posts/regresql-extension/)**, running checks from inside the database
- **[Fixturize](https://github.com/boringSQL/fixturize)** and **[dryrun](https://github.com/boringSQL/dryrun)**, companion tools in the suite


## License

[BSD-2-Clause](LICENSE)
