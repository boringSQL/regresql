# pg_regresql

PostgreSQL extension to force planner to use `pg_class` statistics instead of the estimates from the physical file size. This provides last-mile for RegreSQL query regression testing.

## The Problem

As descibed in article [Production query plans without production data](https://boringsql.com/posts/portable-stats/) planner ignores the `relpages`/`reltuples` stored in `pg_class` when checking the relation size.

Instead it calls `smgrnblocks()` to get the actual physical file size and scales statistics proportionally.

The reasononing for this is to avoid stale statistics (which can happen for example after TRUNCATE). The decision seems to be to use disk size being more reliable than potentially outdated catalog stats.

## What pg_regresql Overrides

The extension hooks into `get_relation_info_hook` (fires after `estimate_rel_size()`) and replaces the planner's physical-size estimates with catalog values.

| Planner field | Default source | pg_regresql source |
|---|---|---|
| `rel->pages` | `smgrnblocks()` via tableam | `pg_class.relpages` |
| `rel->tuples` | density × physical pages | `pg_class.reltuples` |
| `rel->allvisfrac` | `relallvisible` / physical pages | `pg_class.relallvisible / relpages` |
| `IndexOptInfo->pages` | `RelationGetNumberOfBlocks()` | `pg_class.relpages` (index) |
| `IndexOptInfo->tuples` | copied from `rel->tuples` | `pg_class.reltuples` (index) |

## Installation

### From PGXN

```bash
sudo pgxn install pg_regresql
```

### From PostgreSQL source tree

```bash
# Point PG_SOURCE at your PostgreSQL source (must be configured)
make PG_SOURCE=/path/to/postgresql
make install PG_SOURCE=/path/to/postgresql
```

### With PGXS (installed PostgreSQL)

```bash
make USE_PGXS=1
make install USE_PGXS=1
```

## Usage

```
LOAD 'pg_regresql';

EXPLAIN SELECT ..
```

For all sessions on a test instance, add to `postgresql.conf`:

```
session_preload_libraries = 'pg_regresql'
```

## Restoring statistics on PostgreSQL 16 and 17

`CREATE EXTENSION pg_regresql;` adds the two `pg_restore_*` functions pg_dump 18
and later call. Then load the dump, or just its statistics:

    pg_dump --statistics-only -f stats.sql ...
    psql -f stats.sql

On 16, drop the `SET transaction_timeout = 0;` line first. Extended statistics
are not restored: 16/17 reject the `pg_dependencies` literal pg_dump 19 emits,
so skip those statements. From 2.0, drop and recreate the extension; there is no
update script. On 18+ nothing is created, core has the functions.

## Use Cases

The primary use case is SQL query plan regression testing. Inject production statistics into a CI/test database and compare `EXPLAIN` output across schema migrations or PostgreSQL upgrades.

```sql
-- restore the stats
SELECT pg_restore_relation_stats(
    'schemaname', 'public',
    'relname', 'test_orders',
    'relpages', 123513::integer,
    'reltuples', 50000000::real,
    'relallvisible', 123513::integer
);

EXPLAIN SELECT * FROM test_orders WHERE created_at > '2024-06-01';
```

Other use cases are:
- reproducing production query plans locally
- simulating table growth and index strategy (what if scenarios)
- partition planing

## Compatibility

- PostgreSQL 14+ for the planner hook (`get_relation_info_hook`)
- Stats restore functions: 16 and 17 only; 18+ has them in core
- Should work with `pg_hint_plan`, `hypopg`, and other hook-based extensions (not tested for now)

## License

BSD 2-Clause License

Copyright (c) 2026 Radim Marek <radim@boringsql.com>
