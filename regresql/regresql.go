package regresql

import (
	"fmt"
	"os"
)

/*
Init initializes a code repository for RegreSQL processing.

That means creating the ./regresql/ directory, walking the code repository
in search of *.sql files, and creating the associated empty plan files. If
the plan files already exists, we simply skip them, thus allowing to run
init again on an existing repository to create missing plan files.
*/
func Init(root string, pguri string) {
	if err := TestConnectionString(pguri); err != nil {
		fmt.Print(err.Error())
		os.Exit(2)
	}

	suite := Walk(root)

	suite.createRegressDir()
	suite.setupConfig(pguri)

	if err := suite.initRegressHierarchy(); err != nil {
		fmt.Print(err.Error())
		os.Exit(11)
	}

	fmt.Println("")
	fmt.Println("Added the following queries to the RegreSQL Test Suite:")
	suite.Println()

	fmt.Println("")
	fmt.Printf(`Empty test plans have been created in '%s'.
Edit the plans to add query binding values, then run

  regresql update

to create the expected regression files for your test plans. Plans are
simple YAML files containing multiple set of query parameter bindings. The
default plan files contain a single entry named "1", you can rename the test
case and add a value for each parameter.`,
		suite.PlanDir)
}

// PlanQueries create query plans for queries found in the root repository
func PlanQueries(root string, runFilter string) {
	suite := Walk(root)
	suite.SetRunFilter(runFilter)
	config, err := suite.readConfig()

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(3)
	}

	if err := TestConnectionString(config.PgUri); err != nil {
		fmt.Print(err.Error())
		os.Exit(2)
	}

	if err := suite.initRegressHierarchy(); err != nil {
		fmt.Print(err.Error())
		os.Exit(11)
	}

	fmt.Println("")
	fmt.Println("The RegreSQL Test Suite now contains:")
	suite.Println()

	fmt.Println("")
	fmt.Printf(`Empty test plans have been created.
Edit the plans to add query binding values, then run

  regresql update

to create the expected regression files for your test plans. Plans are
simple YAML files containing multiple set of query parameter bindings. The
default plan files contain a single entry named "1", you can rename the test
case and add a value for each parameter. `)
}

/*
Update updates the expected files from the queries and their parameters.
*/
func Update(root string, runFilter string) {
	suite := Walk(root)
	suite.SetRunFilter(runFilter)
	config, err := suite.readConfig()

	if err != nil {
		fmt.Print(err.Error())
		os.Exit(3)
	}

	if err := TestConnectionString(config.PgUri); err != nil {
		fmt.Print(err.Error())
		os.Exit(2)
	}

	if err := suite.createExpectedResults(config.PgUri); err != nil {
		fmt.Print(err.Error())
		os.Exit(12)
	}

	fmt.Println("")
	fmt.Println(`Expected files have now been created.
You can run regression tests for your SQL queries with the command

  regresql test

When you add new queries to your code repository, run 'regresql plan' to
create the missing test plans, edit them to add test parameters, and then
run 'regresql update' to have expected data files to test against.

If you change the expected result set (because picking a new data set or
because new requirements impacts the result of existing queries, you can run
the regresql update command again to reset the expected output files.
 `)
}

func Test(root, runFilter, formatName, outputPath string) {
	suite := Walk(root)
	suite.SetRunFilter(runFilter)
	config, err := suite.readConfig()
	if err != nil {
		fmt.Print(err.Error())
		os.Exit(3)
	}

	if err := TestConnectionString(config.PgUri); err != nil {
		fmt.Print(err.Error())
		os.Exit(2)
	}

	if formatName == "" {
		formatName = "console"
	}
	formatter, err := GetFormatter(formatName)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		os.Exit(14)
	}

	if err := suite.testQueries(config.PgUri, formatter, outputPath); err != nil {
		fmt.Print(err.Error())
		os.Exit(13)
	}
}

// List walks a repository, builds a Suite instance and pretty prints it.
func List(dir string) {
	suite := Walk(dir)
	suite.Println()
}
