# Getting started with RegreSQL

This guide sets up RegreSQL on a throwaway database and covers the whole loop:
output tests, plan tests, and CI. Every step is what you'd do against your own
database.

## The loop

RegreSQL runs every `.sql` file in your project, saves what each query returns
as an expected result you commit to git, then on later runs re-runs the queries
and compares. It catches two kinds of regression: the rows changed, and the
query plan changed (a dropped index, a sequential scan where you had an index
scan).

## What you need

- `regresql` on your PATH (see the [README](../README.md) to install)
- a PostgreSQL to point at

If you don't have one handy, a throwaway in Docker works:

```bash
docker run -d --name rq-shop \
  -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app -e POSTGRES_DB=shop \
  -p 5432:5432 postgres:18
```

## 1. A schema and a query

Load a small schema:

```bash
psql postgres://app:app@localhost:5432/shop <<'SQL'
create table users  (id int primary key, name text, active boolean not null);
create table orders (id int primary key, user_id int not null references users(id),
                     total numeric(10,2) not null);
create index orders_user_id_idx on orders(user_id);
insert into users  values (1,'Ada',true),(2,'Grace',true),(3,'Alan',false);
insert into orders values (1,1,20.00),(2,1,35.50),(3,2,12.00),(4,1,8.25),(5,2,99.99);
SQL
```

Write one query to a file, `orders-by-user.sql`:

```sql
-- name: orders-by-user
select id, total
  from orders
 where user_id = :user_id
order by id;
```

The `:user_id` is a parameter. You'll give it a value in a moment.

## 2. Point RegreSQL at the database

```bash
regresql init postgres://app:app@localhost:5432/shop
```

```
Initialized RegreSQL in ./regresql/
```

This writes `regresql/regress.yaml` with your connection string. It connects
right away, so the database has to be reachable. Now add the query:

```bash
regresql add orders-by-user.sql
```

```
Creating Plan 'regresql/plans/orders-by-user.yaml'
Added 1 plan files
```

`add` creates a plan file, one per query, with a slot for each parameter:

```yaml
# regresql/plans/orders-by-user.yaml
"1":
  user_id: ""
```

RegreSQL doesn't guess parameter values, you set them. Put in a user id:

```yaml
"1":
  user_id: 1
```

Each top-level key is one test case. `"1"` runs the query with `user_id = 1` and
writes its result to `expected/orders-by-user.1.json`, which is where the `.1` in
the test output comes from. Add a `"2":` block to test the same query with
another value.

## 3. Capture the expected output

```bash
regresql update
```

This runs the query and saves what it returns as the expected result:

```json
// regresql/expected/orders-by-user.1.json
{
  "columns": ["id", "total"],
  "rows": [
    [1, "20.00"],
    [2, "35.50"],
    [4, "8.25"]
  ]
}
```

Commit that file. It's the known-good answer. Now run the tests:

```bash
regresql test
```

```
Running regression tests...

RESULTS:
  ✓ 1 passing
```

Green, because the query still returns what's in the expected file.

## 4. Catch a change

Change the query so it returns something different, say a 10% surcharge on
`total`:

```sql
select id, round(total * 1.1, 2) as total
```

```bash
regresql test
```

```
FAILING:
  orders-by-user.1.json
  COMPARISON SUMMARY:
  ├─ Expected: 3 rows
  ├─ Actual:   3 rows
  ├─ Matching: 0 rows
  └─ Modified: 3 rows
  MODIFIED ROWS (showing 3 of 3):
  Row #1:
    Expected: {id: 1, total: "20.00"}
    Actual:   {id: 1, total: "22.00"}
  ...
```

The test fails with the exact before and after. If the change was a mistake, you
fix the query. If it's intended, you accept the new output and commit it:

```bash
regresql update orders-by-user.sql
```

Put the query back the way it was before moving on.

## 5. Catch a slow-down

Wrong results are one kind of regression. A query that still returns the right
rows but got slower is another, and normal tests miss it. Capture a plan
baseline:

```bash
regresql baseline
```

```
Created baseline: orders-by-user.1.json
```

This records the `EXPLAIN` plan and cost. Now drop the index the lookup relies
on:

```bash
psql postgres://app:app@localhost:5432/shop -c "DROP INDEX orders_user_id_idx;"
regresql test
```

```
FAILING:
  orders-by-user.1.cost (362.59 > 121.13 * 110%, +199.3%)
  Expected cost: 121.13
  Actual cost:   362.59 (+199.3%)
  ⚠️ Table 'orders': Bitmap Heap Scan using orders_user_id_idx → Seq Scan
```

The output is identical, so a plain `test` would pass. The plan check catches it:
the query cost tripled and the index scan became a sequential scan. Recreate the
index to make it green again.

## 6. Run it in CI

`regresql test` exits non-zero on a failure, so a CI runner fails the build on a
bad change before it merges. A GitHub Actions job:

```yaml
# .github/workflows/regresql.yml
name: regresql
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    env:
      DATABASE_URL: postgres://app:app@localhost:5432/shop
    services:
      postgres:
        image: postgres:18
        env: { POSTGRES_USER: app, POSTGRES_PASSWORD: app, POSTGRES_DB: shop }
        ports: ["5432:5432"]
        options: >-
          --health-cmd pg_isready --health-interval 10s
          --health-timeout 5s --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.23" }
      - run: go install github.com/boringsql/regresql/v2@latest
      - run: psql "$DATABASE_URL" -f db/schema.sql   # load your schema + data
      - run: regresql test --format github-actions
```

`DATABASE_URL` overrides the `pguri` in `regress.yaml`, so the same project runs
against the CI database without editing the committed config. `--format
github-actions` turns each failure into an inline PR annotation.

CI needs the database in a known state. Loading a schema file works for a small
project; for a real dataset, commit a snapshot with `regresql snapshot build` and
`regresql snapshot restore` before the test step. (You saw the "no snapshot
metadata" note earlier, that's what it's about.)

## Your own queries

To use this on a real project: drop your `.sql` files in, `regresql add` them,
set parameter values in the plan files, `regresql update` to capture expected
output, and `regresql test`. Commit `regresql/` alongside your queries.

- Queries generated by an ORM, so you have no `.sql` files? [qshape](https://github.com/boringSQL/qshape) reads them from `pg_stat_statements` and generates the RegreSQL skeletons.
- A complete worked project to read: [examples/cdstore](../examples/cdstore/README.md).
- Output formats, snapshots, migrations, cross-version testing: the [main README](../README.md).
