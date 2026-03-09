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

- PostgreSQL 14+ (uses `get_relation_info_hook`, available since PG 8.3)
- Should work with `pg_hint_plan`, `hypopg`, and other hook-based extensions (not tested for now)

## License

BSD 2-Clause License

Copyright (c) 2026 Radim Marek <radim@boringsql.com>
