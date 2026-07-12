# CD Store Example

A working RegreSQL project built on the Chinook music-store database. The queries come from "The Art of PostgreSQL" by Dimitri Fontaine.

## Overview

It covers the snapshot build pipeline (schema plus SQL fixtures), queries that use window functions, lateral joins and aggregations, test plans with multiple parameter bindings, and a schema of artists, albums, tracks, genres and playlists.

## Database Schema

The Chinook database models a digital media store:

```
artist (artist_id, name)
  |
album (album_id, title, artist_id, created_at)
  |
track (track_id, name, album_id, genre_id, milliseconds, bytes, unit_price)
  |
genre (genre_id, name)
playlist_track (playlist_id, track_id)
```

## Setup

### 1. Create Database

```bash
createdb cdstore
```

### 2. Build Snapshot

The snapshot pipeline applies the schema and loads SQL fixtures in one step:

```bash
cd examples/cdstore
regresql snapshot build
```

This runs `db/schema.sql` then each file in `db/fixtures/` to create `snapshots/default.dump`.

### 3. Restore and Test

```bash
regresql snapshot restore   # restore snapshot into test DB
regresql update             # generate expected output files
regresql test               # run all tests
```

## Queries

### artist.sql - Top Artists by Album Count

```sql
-- name: top-artists-by-album
select artist.name, count(*) as albums
  from artist left join album using(artist_id)
group by artist.name
order by albums desc, artist.name
limit :n;
```

### album-by-artist.sql - Albums by Artist with Duration

```sql
-- name: list-albums-by-artist
select album.title as album,
       created_at,
       sum(milliseconds) * interval '1 ms' as duration
  from album
       join artist using(artist_id)
       left join track using(album_id)
 where artist.name = :name
group by album, created_at
order by album;
```

### album-tracks.sql - Track List with Cumulative Duration

```sql
-- name: list-tracks-by-albumid
select name as title,
       milliseconds * interval '1ms' as duration,
       (sum(milliseconds) over (order by track_id) - milliseconds)
         * interval '1ms' as "begin",
       sum(milliseconds) over (order by track_id)
         * interval '1ms' as "end",
       round(milliseconds * 100.0 / sum(milliseconds) over (), 2) as pct
  from track
 where album_id = :album_id
order by track_id;
```

Uses window functions (`sum() over`), interval arithmetic, and a per-track percentage of the album total.

### genre-tracks.sql - Track Count by Genre

```sql
-- name: tracks-by-genre
select genre.name, count(*) as count
  from genre left join track using(genre_id)
group by genre.name
order by count desc;
```

### genre-topn.sql - Top N Tracks per Genre

Advanced query using LATERAL joins:

```sql
-- name: genre-top-n
select genre.name as genre,
       case when length(ss.name) > 15
            then substring(ss.name from 1 for 15) || '...'
            else ss.name
        end as track,
       artist.name as artist
  from genre
       left join lateral (...) ss on true
       join album using(album_id)
       join artist using(artist_id)
order by genre.name, ss.count desc, ss.name;
```

Uses a LATERAL join for Top-N per group, weighting tracks by how often they appear in a playlist.

## Try It

```bash
regresql snapshot build     # create snapshot from schema + fixtures
regresql update             # run every query, save expected output
regresql test               # re-run and compare — all green
```

When using `update` only - RegreSQL tests **query output** only - every row and column must match exactly. Try changing `order by albums desc` to `asc` in `artist.sql` and re-test.

`regresql test`

```
FAILING:
  artist_top-artists-by-album.1.json

  COMPARISON SUMMARY:
  ├─ Expected: 5 rows
  ├─ Actual:   5 rows
  ├─ Matching: 0 rows
  └─ Modified: 5 rows

  MODIFIED ROWS (showing 5 of 5):
  Row #1:
    Expected: {name: "AC/DC", albums: 3}
    Actual:   {name: "Accept", albums: 1}
  Row #2:
    Expected: {name: "Led Zeppelin", albums: 3}
    Actual:   {name: "Audioslave", albums: 1}
  Row #3:
    Expected: {name: "Metallica", albums: 3}
    Actual:   {name: "Buddy Guy", albums: 1}
  Row #4:
    Expected: {name: "Foo Fighters", albums: 2}
    Actual:   {name: "Creedence Clearwater Revival", albums: 1}
  Row #5:
    Expected: {name: "Iron Maiden", albums: 2}
    Actual:   {name: "Deep Purple", albums: 1}

To accept changes: regresql update <query-name>  
```

You can either fix the query (regression), or if the query output matches the new business requirement update the expected result.

```
regresql update artist.sql
```

And follow up tests will pass again.

## Baselines

`regresql test` on its own catches output changes. To also catch query plan
changes, an index scan that turns into a sequential scan or a join method that
flips, capture baselines:

```bash
regresql baseline
```

This runs `EXPLAIN` for every query and records the plan: scan types, join
methods, indexes used. Now drop an index one of the queries relies on:

```sql
DROP INDEX track_album_id_idx;
```

```bash
regresql test
```

RegreSQL compares the new plan against the baseline and reports what changed,
with the anti-patterns the change introduced:

```
  ✗ Table 'track': Index Scan using track_album_id_idx → Seq Scan
  ℹ️ Join strategy changed: [Nested Loop, Hash Join] → [Hash Join, Hash Join]

WARNINGS:
  ⚠️  Multiple sequential scans detected on tables: album, playlist_track, track, artist
  ⚠️  Multiple sort operations detected (2 sorts)
  ⚠️  Nested loop join with sequential scan detected
```

On a dataset this small a sequential scan is often cheaper than the index, so
the signal here is the plan shape, not the timing. On production-sized data the
same check is what catches a dropped index or a bad plan change before it ships.

## Running Tests

```bash
regresql test               # run all tests
regresql test --run artist  # run specific query
regresql update             # regenerate expected results
regresql baseline           # recapture plan baselines
```

## SQL Fixtures

Test data lives in `db/fixtures/` as plain SQL files, loaded in order during `snapshot build`:

- `01_base_data.sql` - Artists, genres, media types
- `02_albums.sql` - Albums with tracks
- `03_playlists.sql` - Playlists with track associations

## Directory Structure

```
cdstore/
├── README.md
├── artist.sql                          # SQL query files
├── album-by-artist.sql
├── album-tracks.sql
├── genre-tracks.sql
├── genre-topn.sql
├── db/
│   ├── schema.sql                      # Database schema
│   └── fixtures/                       # SQL fixture data
│       ├── 01_base_data.sql
│       ├── 02_albums.sql
│       └── 03_playlists.sql
├── snapshots/
│   └── default.dump                    # Built snapshot (auto-generated)
└── regresql/
    ├── regress.yaml                    # Configuration
    ├── plans/                          # Test plans (parameter bindings)
    ├── expected/                       # Expected results (auto-generated)
    └── baselines/                      # Query baselines (auto-generated)
```


## Credits

- **Database**: [Chinook Database](https://github.com/lerocha/chinook-database) by Luis Rocha
- **Queries**: Based on examples from [The Art of PostgreSQL](https://theartofpostgresql.com/) by Dimitri Fontaine

## License

Example code released under the BSD-2 License.
