# RegreSQL, Regression Testing your SQL queries

The `regresql` tool implement a regression testing facility for SQL queries,
and supports the PostgreSQL database system. A regression test allows to
ensure known results when the code is edited. To enable that we need:

  - some code to test, here SQL queries
  - a known result set for each SQL query
  - a way to set up test data in a consistent state
  - a regression driver that runs queries again and check their result
    against the known expected result set

The RegreSQL tool is that regression driver. It helps with creating the
expected result set for each query and then running query files again to
check that the results are still the same. It also provides declarative
fixture system for managing test data, making it easy to set up complex
database states with generated or static data.

Of course, for the results the be comparable the queries need to be run
against a known PostgreSQL database content.

## Installing

The `regresql` tool is written in Go. To install it, use:

    go install github.com/boringsql/regresql@latest

This command will compile and install the binary in your `$GOPATH/bin`,
which defaults to `~/go/bin`. Make sure this directory is in your `$PATH`
to run `regresql` from anywhere.

If you're new to Go, see <https://golang.org/doc/install> for installation
instructions and environment setup.

## Basic usage

Basic usage or regresql:

  - `regresql init [ -C dir ]`

    Creates the regresql main directories and runs all SQL queries found in
    your target code base (defaults to current directory).

    The -C option changes current directory to *dir* before running the
    command.

  - `regresql plan [ -C dir ] [ --run pattern ]`

    Create query plan files for all queries. Run that command when you add
    new queries to your repository.

  - `regresql update [ -C dir ] [ --run pattern ]`

    Updates the *expected* files from the queries, considering that the
    output is valid.

  - `regresql baseline [ -C dir]`

    Update the cost baselines (EXPLAIN) for the queries.

  - `regresql test [ -C dir ] [ --run pattern ] [ --format format ] [ -o output ]`

    Runs all the SQL queries found in current directory.

    The -C option changes the current directory before running the tests.

    The --format option specifies output format (default: console):
      - console: Human-readable output with ✓/✗ symbols
      - pgtap: TAP (Test Anything Protocol) format
      - junit: JUnit XML format for CI/CD integration
      - json: Structured JSON output
      - github-actions: GitHub Actions workflow annotations

    The -o option specifies output file path (default: stdout)

  - `regresql baseline [ -C dir ] [ --run pattern ]`

    Creates baseline EXPLAIN analysis for queries.

  - `regresql list [ -C dir ]`

    List all SQL files found in current directory.

    The -C option changes the current directory before listing the files.

The --run option runs only queries matching the given pattern (regexp). The
pattern matches against both file names and query names. Examples:

```
# Run only queries in files or queries containing "user"
$ regresql test --run user

# Run only queries starting with "get" (using regexp)
$ regresql test --run "^get"

# Run specific query by exact name
$ regresql test --run "^list-albums-by-artist$"

# Run queries from specific file
$ regresql test --run "album-tracks.sql"

# Run multiple patterns (using regexp OR)
$ regresql test --run "user|artist"
```

## SQL query files

RegreSQL finds every `*.sql` file in your repository and executes the queries
against PostgreSQL. Each file can contain one or more queries, optionally
annotated with metadata.

Queries are separated and identified using header comments. The basic structure is:
```sql
-- name: query_name
-- metadata: key1=value1, key2=value2
SELECT ...;
```

`-- name`: defines the unique identifier for the query within the file. It is required when a file contains multiple queries. Queries are referenced and executed by this name.

`-- metadata`: allows attaching metadata. Currently supported options:

```sql
-- regresql: notest          Skip running this query in tests
-- regresql: nobaseline      Skip creating a baseline for this query
-- regresql: noseqscanwarn   Suppress sequential scan warnings for this query
```

Multiple options can be combined, separated by commas:

```sql
-- regresql: notest, nobaseline
-- regresql: noseqscanwarn    # Useful for queries that intentionally scan entire tables
```

It's also possible to use a snigle query in a file, without `--name` annotation, in which case the query is automatically named after the file name (without the .sql extension). For example file `my_query.sql`

```sql
SELECT 42;
```

is equivalent to

```sql
name: my_query
SELECT 42
```

Notes:
- Queries can include named parameters (:param_name) or positional parameters ($1, $2).
- Semicolons inside strings or comments are ignored; only the semicolon terminating a query ends the block.
- The query handling is available as a separate [queries](http://github.com/boringSQL/queries) library.

## Test Fixtures

RegreSQL provides a declarative fixture system for managing test data. Fixtures allow you to set up complex database states with static or generated data, making your tests more reliable and maintainable.

### Quick Example

Create a fixture in `regresql/fixtures/users.yaml`:

```yaml
fixture: users
description: Test user accounts
cleanup: rollback
data:
  - table: users
    rows:
      - id: 1
        email: test@example.com
        name: Test User
```

Use it in your test plan `regresql/plans/get-user.yaml`:

```yaml
fixtures:
  - users

"1":
  email: test@example.com
```

The fixture will be automatically loaded before test and cleaned up after.

### Key Features

- **Static data**: Define exact test data in YAML
- **Generated data**: Use built-in generators for realistic test data at scale
- **SQL fixtures**: Execute SQL scripts or inline SQL statements
- **Dependencies**: Fixtures can depend on other fixtures
- **Cleanup strategies**: Rollback (default), truncate, or none
- **Data generators**: sequence, int, decimal, string, email, name, uuid, date_between, and more
- **Advanced generators**: Foreign keys, ranges, patterns, and Go templates

### Fixture Commands

```bash
# List all fixtures
regresql fixtures list

# Validate fixture definitions
regresql fixtures validate [fixture-name]

# Show fixture details and dependencies
regresql fixtures show <fixture-name>

# Apply fixture to database (for debugging)
regresql fixtures apply <fixture-name>

# Show dependency graph
regresql fixtures deps [fixture-name]
```

### Example: Generated Data

Generate 1000 realistic customer records:

```yaml
fixture: large_dataset
generate:
  - table: customers
    count: 1000
    columns:
      id:
        generator: sequence
        start: 1
      email:
        generator: email
        domain: example.com
      name:
        generator: name
        type: full
      created_at:
        generator: date_between
        start: "2023-01-01"
        end: "2024-01-01"
```


## Test Suites

By default a Test Suite is a source directory.

## File organisation

RegreSQL needs the following files and directories to run:

  - `./regresql` where to register needed files

  - `./regresql/regresql.yaml`

    Configuration file for the directory in which it's installed. It
    contains the PostgreSQL connection string where to connect to for
    running the regression tests and the top level directory where to find
    the SQL files to test against.

  - `./regresql/expected/path/to/file_query-name.yaml`

    For each file *file.sql* found in your source tree, RegreSQL creates a
    subpath in `./regresql/plans` with a *file_query-name.yaml* file. This YAML file
    contains query plans: that's a list of SQL parameters values to use when
    testing.

  - `./regresql/expected/path/to/file_query-name.json`

    For each file *query.sql* found in your source tree, RegreSQL creates a
    subpath in `./regresql/expected` directory and stores in *file_query-name.json* the
    expected result set of the query in JSON format,

  - `./regresql/out/path/to/file_query-name.json`

    The result of running the query in *file_query-name.sql* is stored in *query.json*
    in the `regresql/out` directory subpath for it, so that it is possible
    to compare this result to the expected one in `regresql/expected`.

In all cases `query_name` is replaced by the tagged query name. If not present, name
`default` is used.

## Example

In a small local application the command `regresql list` returns the
following SQL source files:

```
$ regresql list
.
  src/sql/
    album-by-artist.sql
    album-tracks.sql
    artist.sql
    genre-topn.sql
    genre-tracks.sql
```

After having done the following commands:

```
$ regresql init postgres:///chinook?sslmode=disable
...

$ regresql update
...
```

Now we have to edit the YAML plan files to add bindings, here's an example
for a query using a single parameter, `:name`:

```
$ cat src/sql/album_by_artist.sql
-- name: list-albums-by-artist
-- List the album titles and duration of a given artist
  select album.title as album,
         sum(milliseconds) * interval '1 ms' as duration
    from album
         join artist using(artistid)
         left join track using(albumid)
   where artist.name = :name
group by album
order by album;

$ cat regresql/plans/src/sql/album_by_artist_album-by-artist.yaml
"1":
  name: "Red Hot Chili Peppers"
```

And we can now run the tests:

```
$ regresql test
Connecting to 'postgres:///chinook?sslmode=disable'… ✓
Running regression tests...

✓ album-by-artist_list-albums-by-artist.2.json (0.00s)
✓ album-by-artist_list-albums-by-artist.1.json (0.00s)
✓ album-by-artist_list-albums-by-artist.2.cost (22.09 <= 22.09 * 110%) (0.00s)
  ⚠️  Sequential scan detected on table 'artist'
    Suggestion: Consider adding an index if this table is large or this query is frequently executed
  ⚠️  Nested loop join with sequential scan detected
    Suggestion: Add index on join column to avoid repeated sequential scans
✓ album-by-artist_list-albums-by-artist.1.cost (22.09 <= 22.09 * 110%) (0.00s)
  ⚠️  Sequential scan detected on table 'artist'
    Suggestion: Consider adding an index if this table is large or this query is frequently executed
  ⚠️  Nested loop join with sequential scan detected
    Suggestion: Add index on join column to avoid repeated sequential scans

✓ album-tracks_list-tracks-by-albumid.1.json (0.00s)
✓ album-tracks_list-tracks-by-albumid.2.json (0.00s)
✓ album-tracks_list-tracks-by-albumid.1.cost (8.23 <= 8.23 * 110%) (0.00s)
✓ album-tracks_list-tracks-by-albumid.2.cost (8.23 <= 8.23 * 110%) (0.00s)

✓ artist_top-artists-by-album.1.json (0.00s)
✓ artist_top-artists-by-album.1.cost (35.70 <= 35.70 * 110%) (0.00s)
  ⚠️  Multiple sequential scans detected on tables: album, artist
    Suggestion: Review query and consider adding indexes on filtered/joined columns

✓ genre-topn_genre-top-n.top-1.json (0.00s)
✓ genre-topn_genre-top-n.top-3.json (0.00s)
✓ genre-topn_genre-top-n.top-1.cost (6610.59 <= 6610.59 * 110%) (0.00s)
  ⚠️  Multiple sequential scans detected on tables: artist, genre
    Suggestion: Review query and consider adding indexes on filtered/joined columns
  ⚠️  Multiple sort operations detected (2 sorts)
    Suggestion: Consider composite indexes for ORDER BY clauses to avoid sorting
  ⚠️  Nested loop join with sequential scan detected
    Suggestion: Add index on join column to avoid repeated sequential scans
✓ genre-topn_genre-top-n.top-3.cost (6610.59 <= 6610.59 * 110%) (0.00s)
  ⚠️  Multiple sequential scans detected on tables: genre, artist
    Suggestion: Review query and consider adding indexes on filtered/joined columns
  ⚠️  Multiple sort operations detected (2 sorts)
    Suggestion: Consider composite indexes for ORDER BY clauses to avoid sorting
  ⚠️  Nested loop join with sequential scan detected
    Suggestion: Add index on join column to avoid repeated sequential scans

✓ genre-tracks_tracks-by-genre.json (0.00s)
✓ genre-tracks_tracks-by-genre.cost (37.99 <= 37.99 * 110%) (0.00s)
  ⚠️  Multiple sequential scans detected on tables: genre, track
    Suggestion: Review query and consider adding indexes on filtered/joined columns

Results: 16 passed (0.00s)
```

We can see the following files have been created by the RegreSQL tool:

```
$ tree regresql/
regresql/
├── baselines
│   └── src
│       └── sql
│           ├── album-by-artist_album-by-artist.1.json
│           ├── album-tracks_album-tracks.1.json
│           ├── artist_top-artists-by-album.1.json
│           ├── genre-topn_genre-top-n.top-1.json
│           ├── genre-topn_genre-top-n.top-3.json
│           └── genre-tracks_tracks-by-genre.1.json
├── expected
│   └── src
│       └── sql
│           ├── album-by-artist.1.json
│           ├── album-tracks.1.json
│           ├── artist.1.json
│           ├── genre-topn.1.json
│           ├── genre-topn.top-1.json
│           ├── genre-topn.top-3.json
│           └── genre-tracks.json
├── out
│   └── src
│       └── sql
│           ├── album-by-artist.1.json
│           ├── album-tracks.1.json
│           ├── artist.1.json
│           ├── genre-topn.1.json
│           ├── genre-topn.top-1.json
│           ├── genre-topn.top-3.json
│           └── genre-tracks.json
├── plans
│   └── src
│       └── sql
│           ├── album-by-artist.yaml
│           ├── album-tracks.yaml
│           ├── artist.yaml
│           └── genre-topn.yaml
└── regress.yaml

12 directories, 27 files
```

## History

The project is a fork of original `regresql` written by [Dimitri Fontaine](https://github.com/dimitri) as part his book [Mastering PostgreSQL](http://masteringpostgresql.com/). The tool was originally inspired by PostgreSQL’s own regression testing framework, providing a lightweight and SQL-native approach to unit and regression testing.

The fork’s goal is to extend RegreSQL into a modern, extensible framework that supports the broader [boringSQL](https://boringsql.com) vision - helping developers to feel more confident working with SQL queries.

## License

The RegreSQL utility is released under [The PostgreSQL License](https://www.postgresql.org/about/licence/).
