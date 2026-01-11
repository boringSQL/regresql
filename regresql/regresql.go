package regresql

import (
	"fmt"
	"os"
	"time"
)

type TestOptions struct {
	Root          string
	RunFilter     string
	FormatName    string
	OutputPath    string
	Commit        bool
	NoRestore     bool
	ForceRestore  bool
	FailOnSkipped bool
	Color         bool
	NoColor       bool
	FullDiff      bool
	NoDiff        bool
}

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

	suite := Walk(root, []string{})

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
	config, err := ReadConfig(root)
	ignorePatterns := []string{}
	if err == nil {
		ignorePatterns = config.Ignore
	}

	suite := Walk(root, ignorePatterns)
	suite.SetRunFilter(runFilter)
	config, err = suite.readConfig()
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
Each query runs in its own transaction that rolls back (unless commit is true).
*/
func Update(root string, runFilter string, commit, noRestore, forceRestore bool) {
	config, err := ReadConfig(root)
	ignorePatterns := []string{}
	if err == nil {
		ignorePatterns = config.Ignore
	}

	suite := Walk(root, ignorePatterns)
	suite.SetRunFilter(runFilter)
	config, err = suite.readConfig()
	if err != nil {
		fmt.Print(err.Error())
		os.Exit(3)
	}

	autoRestore(config, root, noRestore, forceRestore)

	// Validate schema hasn't changed since last snapshot build
	if err := ValidateSchemaHash(root); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	// Validate migrations haven't changed since last snapshot build
	if err := ValidateMigrationsHash(root); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	// Validate migration command hasn't changed since last snapshot build
	if err := ValidateMigrationCommandHash(root); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	if err := TestConnectionString(config.PgUri); err != nil {
		fmt.Print(err.Error())
		os.Exit(2)
	}

	// Validate server settings match snapshot (warn, strict, or ignore)
	if err := validateServerSettings(config, root); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	if err := suite.createExpectedResults(config.PgUri, commit); err != nil {
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

func autoRestore(cfg config, root string, noRestore, forceRestore bool) {
	if noRestore || !ShouldAutoRestore(cfg.Snapshot) {
		return
	}
	snapshotPath := GetSnapshotPath(cfg.Snapshot, root)
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		fmt.Printf("Error: snapshot file not found: %s\n\nRun 'regresql snapshot build' to create a snapshot, or use '--no-restore' to skip\n", snapshotPath)
		os.Exit(1)
	}

	snapshotsDir := GetSnapshotsDir(root)
	targetDB := cfg.Snapshot.RestoreDatabase

	if !forceRestore {
		needsRestore, reason := NeedsRestore(snapshotsDir, snapshotPath, targetDB)
		if !needsRestore {
			state, _ := ReadRestoreState(snapshotsDir)
			fmt.Printf("Skipping restore: snapshot unchanged since %s (restored in %.1fs)\n\n",
				state.RestoredAt.Local().Format("2006-01-02 15:04:05"),
				float64(state.DurationMillis)/1000)
			return
		}
		fmt.Printf("Restoring snapshot: %s (%s)\n", snapshotPath, reason)
	} else {
		fmt.Printf("Restoring snapshot: %s (forced)\n", snapshotPath)
	}

	start := time.Now()
	opts := RestoreOptions{
		InputPath:      snapshotPath,
		Clean:          true,
		TargetDatabase: targetDB,
	}
	if err := RestoreSnapshot(cfg.PgUri, opts); err != nil {
		fmt.Printf("Error: failed to restore snapshot: %s\n", err)
		os.Exit(1)
	}
	duration := time.Since(start)

	stat, _ := os.Stat(snapshotPath)
	state := &RestoreState{
		SnapshotPath:   snapshotPath,
		SnapshotMtime:  stat.ModTime(),
		SnapshotSize:   stat.Size(),
		Database:       targetDB,
		RestoredAt:     time.Now().UTC(),
		DurationMillis: duration.Milliseconds(),
	}
	WriteRestoreState(snapshotsDir, state)

	fmt.Printf("Restored in %.1fs\n\n", duration.Seconds())
}

func validateServerSettings(cfg config, root string) error {
	mode := GetValidateSettings(cfg.Snapshot)
	if mode == ValidateSettingsIgnore {
		return nil
	}

	snapshotsDir := GetSnapshotsDir(root)
	metadata, err := ReadSnapshotMetadata(snapshotsDir)
	if err != nil {
		return nil // no metadata - nothing to validate
	}

	if metadata.Current == nil || metadata.Current.Server == nil {
		return nil // no server context stored
	}

	db, err := OpenDB(cfg.PgUri)
	if err != nil {
		return fmt.Errorf("failed to connect for server validation: %w", err)
	}
	defer db.Close()

	validation, err := ValidateServerContext(db, metadata.Current.Server)
	if err != nil {
		return fmt.Errorf("failed to validate server context: %w", err)
	}

	if !validation.HasDifferences() {
		return nil
	}

	warning := validation.FormatWarning()

	if mode == ValidateSettingsStrict {
		return fmt.Errorf("%s\n\nUse validate_settings: warn to continue with warnings", warning)
	}

	fmt.Printf("%s\n\n", warning)
	return nil
}

// Test runs regression tests for all queries.
// Each query runs in its own transaction that rolls back (unless commit is true).
//
// Exit codes:
//
//	0  - all tests passed
//	1  - test failures
//	2  - skipped tests (if failOnSkipped)
//	3  - config error
//	13 - query execution error
//	14 - invalid formatter
func Test(opts TestOptions) {
	config, err := ReadConfig(opts.Root)
	ignorePatterns := []string{}
	if err == nil {
		ignorePatterns = config.Ignore
	}

	suite := Walk(opts.Root, ignorePatterns)
	suite.SetRunFilter(opts.RunFilter)
	config, err = suite.readConfig()
	if err != nil {
		fmt.Print(err.Error())
		os.Exit(3)
	}

	// Cache config for plan quality analysis
	SetGlobalConfig(config)

	autoRestore(config, opts.Root, opts.NoRestore, opts.ForceRestore)

	// Validate schema hasn't changed since last snapshot build
	if err := ValidateSchemaHash(opts.Root); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	// migrations haven't changed since last snapshot build?:
	if err := ValidateMigrationsHash(opts.Root); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	// Validate migration command hasn't changed since last snapshot build
	if err := ValidateMigrationCommandHash(opts.Root); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	if err := TestConnectionString(config.PgUri); err != nil {
		fmt.Print(err.Error())
		os.Exit(2)
	}

	// Validate server settings match snapshot (warn, strict, or ignore)
	if err := validateServerSettings(config, opts.Root); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	formatName := opts.FormatName
	if formatName == "" {
		formatName = "console"
	}
	formatter, err := GetFormatter(formatName)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		os.Exit(14)
	}

	// Configure console formatter options
	if cf, ok := formatter.(*ConsoleFormatter); ok {
		cf.SetOptions(ConsoleOptions{
			Color:    opts.Color,
			NoColor:  opts.NoColor,
			FullDiff: opts.FullDiff,
			NoDiff:   opts.NoDiff,
		})
	}

	summary, err := suite.testQueries(config.PgUri, formatter, opts.OutputPath, opts.Commit)
	if err != nil {
		fmt.Print(err.Error())
		os.Exit(13)
	}
	if summary.Failed > 0 {
		os.Exit(1)
	}
	if opts.FailOnSkipped && summary.Skipped > 0 {
		os.Exit(2)
	}
}

// List walks a repository, builds a Suite instance and pretty prints it.
func List(dir string) {
	config, err := ReadConfig(dir)
	ignorePatterns := []string{}
	if err == nil {
		ignorePatterns = config.Ignore
	}

	suite := Walk(dir, ignorePatterns)
	suite.Println()
}
