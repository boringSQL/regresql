# RegreSQL, Regression Testing your SQL queries

The `regresql` tool implement a regression testing facility for SQL queries,
and supports the PostgreSQL database system. A regression test allows to
ensure known results when the code is edited. To enable that we need:

  - some code to test, here SQL queries
  - a known result set for each SQL query,
  - a regression driver that runs queries again and check their result
    against the known expected result set.

The RegreSQL tool is that regression driver. It helps with creating the
expected result set for each query and then running query files again to
check that the results are still the same.

Of course, for the results the be comparable the queries need to be run
against a known PostgreSQL database content.

## Installing

The `regresql` tool is written in Go, so:

    go get github.com/boringsql/regresql

This command will compile and install the command in your `$GOPATH/bin`,
which defaults to `~/go/bin`. See <https://golang.org/doc/install> if you're
new to the Go language.

## Basic usage

Basic usage or regresql:

  - `regresql init [ -C dir ]`

    Creates the regresql main directories and runs all SQL queries found in
    your target code base (defaults to current directory).

    The -C option changes current directory to *dir* before running the
    command.

  - `regresql plan [ -C dir ]`

    Create query plan files for all queries. Run that command when you add
    new queries to your repository.

  - `regresql update [ -C dir ]`

    Updates the *expected* files from the queries, considering that the
    output is valid.

  - `regresql test [ -C dir ]`

    Runs all the SQL queries found in current directory.

    The -C option changes the current directory before running the tests.

  - `regresql list [ -C dir ]`

    List all SQL files found in current directory.

    The -C option changes the current directory before listing the files.

## SQL query files

RegreSQL finds every *.sql* file in your code repository and runs them
against PostgreSQL. It means you're supposed to maintain your queries as
separate query files, see the
excellent <https://github.com/krisajenkins/yesql> Clojure library to see how
that's done. The project links to many implementation in other languages,
including Python, PHP or Go.

SQL files might contain variables, and RegreSQL implements the same support
for them as `psql`, see the PostgreSQL documentation
about
[psql variables](https://www.postgresql.org/docs/current/static/app-psql.html#APP-PSQL-VARIABLES) and
their usage syntax and quoting rules: `:foo`, `:'foo'` and `:"foo"`.

RegreSQL supports either single query per SQL file, or multiple queries in file. In latter case you have
to tag/name the queries to enable the support.

Example

```
-- name: my-sample-query
SELECT count(*) FROM users
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

  - `./regresql/expected/path/to/file_query-name.out`

    For each file *query.sql* found in your source tree, RegreSQL creates a
    subpath in `./regresql/expected` directory and stores in *file_query-name.out* the
    expected result set of the query,

  - `./regresql/out/path/to/file_query-name.sql`

    The result of running the query in *file_query-name.sql* is stored in *query.out*
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
TAP version 13
ok 1 - src/sql/album-by-artist_album-by-artist.1.out
ok 2 - src/sql/album-tracks_album-tracks.1.out
ok 3 - src/sql/artist_top-artists-by-album.1.out
ok 4 - src/sql/genre-topn_genre-top-n.top-3.out
ok 5 - src/sql/genre-topn.genre-top-n.top-1.out
ok 6 - src/sql/genre-tracks_tracks-by-genre.1.out
```

We can see the following files have been created by the RegreSQL tool:

```
$ tree regresql/
regresql/
├── expected
│   └── src
│       └── sql
│           ├── album-by-artist.1.out
│           ├── album-tracks.1.out
│           ├── artist.1.out
│           ├── genre-topn.1.out
│           ├── genre-topn.top-1.out
│           ├── genre-topn.top-3.out
│           └── genre-tracks.out
├── out
│   └── src
│       └── sql
│           ├── album-by-artist.1.out
│           ├── album-tracks.1.out
│           ├── artist.1.out
│           ├── genre-topn.1.out
│           ├── genre-topn.top\ 1.out
│           ├── genre-topn.top\ 3.out
│           ├── genre-topn.top-1.out
│           ├── genre-topn.top-3.out
│           └── genre-tracks.out
├── plans
│   └── src
│       └── sql
│           ├── album-by-artist.yaml
│           ├── album-tracks.yaml
│           ├── artist.yaml
│           └── genre-topn.yaml
└── regress.yaml

9 directories, 21 files
```

## History

This tool is inspired by the PostgreSQL regression testing framework. It's
been written in the process of
the [Mastering PostgreSQL](http://masteringpostgresql.com/) book as an
example of an SQL framework for unit testing and regression testing.

## License

The RegreSQL utility is released
under [The PostgreSQL License](https://www.postgresql.org/about/licence/).
