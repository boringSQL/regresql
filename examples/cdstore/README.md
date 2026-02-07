# CD Store Example

Complete working example demonstrating RegreSQL with the Chinook database (a sample music store database). This example is based on the queries from "The Art of PostgreSQL" book.

## Overview

This example demonstrates:
- **Snapshot build pipeline** with schema + SQL fixtures
- **Complex SQL queries** with window functions, lateral joins, and aggregations
- **Test plans** with multiple parameter bindings
- **Real-world schema** (artists, albums, tracks, genres, playlists)

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
order by albums desc
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
       round(milliseconds / sum(milliseconds) over () * 100, 2) as pct
  from track
 where album_id = :album_id
order by track_id;
```

**Features**: window functions (`sum() over`), interval arithmetic, percentage calculations

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
order by genre.name, ss.count desc;
```

**Features**: LATERAL joins for Top-N per group, playlist-based popularity weighting

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
    Actual:   {name: "Pearl Jam", albums: 1}
  Row #2:
    Expected: {name: "Metallica", albums: 3}
    Actual:   {name: "Jamiroquai", albums: 1}
  Row #3:
    Expected: {name: "Led Zeppelin", albums: 3}
    Actual:   {name: "Accept", albums: 1}
  Row #4:
    Expected: {name: "Pink Floyd", albums: 2}
    Actual:   {name: "Nirvana", albums: 1}
  Row #5:
    Expected: {name: "Radiohead", albums: 2}
    Actual:   {name: "Lenny Kravitz", albums: 1}

To accept changes: regresql update <query-name>  
```

You can either fix the query (regression), or if the query output matches the new business requirement update the expected result.

```
regresql update artist.sql
```

And follow up tests will pass again.

## Baselines

In previous example `regresql test` catches content only changes. To also catch **query plan regressions**
(a dropped index, a seq scan that used to be an index scan), all you need to do is capture baselines:

```bash
regresql baseline
```

This runs `EXPLAIN` for every query and saves the plan signature — scan types,
join methods, indexes used. Now try dropping an index:

```sql
DROP INDEX track_album_id_idx;
```

```bash
regresql test
```

Despite all the tests passing, RegreSQL is able to detect sub-optimal plans.

```
WARNINGS:
  genre-topn_genre-top-n.top-1.cost (20.82 <= 20.82 * 110%)
  ⚠️  Multiple sort operations detected (2 sorts)
    Suggestion: Consider composite indexes for ORDER BY clauses to avoid sorting
  genre-topn_genre-top-n.top-3.cost (21.79 <= 21.79 * 110%)
  ⚠️  Multiple sort operations detected (2 sorts)
    Suggestion: Consider composite indexes for ORDER BY clauses to avoid sorting
```

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
