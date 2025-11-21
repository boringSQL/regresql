/*
regresql package implements the RegreSQL commands.

The main entry point of this package is a Suite data structure instance,
which can be obtained with the Walk() function:

    func List(dir string) {
    	suite := Walk(dir, []string{})
    	suite.Println()
    }

That's the simplest you can do with a suite instance, and that's the whole
implementation of the exported List function too.

One you have a test suite instance, for more interesting things you usually
want to read regresql configuration which is created by the Init() command
and stores the PostgreSQL connection string, in the format expected by the
github.com/lib/pq library:

	suite := Walk(root, []string{})
	config, err := suite.readConfig()

	if err != nil {
		fmt.Printf(err.Error())
		os.Exit(3)
	}

	if err := TestConnectionString(config.PgUri); err != nil {
		fmt.Printf(err.Error())
		os.Exit(2)
	}

Now that you have a test suite and a valid PostgreSQL connection string,
it's possible to run the SQL queries. A query is typically as in the
following example:

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

This is parsed to find out the parameters, spelled in a `psql` compatible
way as documented in
https://www.postgresql.org/docs/9.6/static/app-psql.html#APP-PSQL-VARIABLES.

To be able to run regression tests against such a query with parameters, we
need parameter values. That's to be found in a Plan file. A plan file is a
YAML file associated with a query, such as the following:

    "1":
      name: "Red Hot Chili Peppers"

In this file we find a single implementation of the query parameters, named
"1" (that's automatically filled in by the Init() function). Our user edited
the file to fill in "Red Hot Chili Peppers" from an empty string, as created
by the Init() function.

The Init() function created a YAML plan file for each query, using the
https://github.com/spf13/viper library. The user is expected to edit the
YAML files. Once the parameters are edited it's possible to run the queries.

Update() runs the queries and stores their results in an expected file.

Test() runs the queries, stores their results in an out file (the actual
output) and compares this actual result set with the expected one.
*/
package regresql
