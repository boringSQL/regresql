package regresql

import (
	"fmt"
	"os"
	"time"
)

type (
	TestOptions struct {
		Root          string
		RunFilter     string
		FormatName    string
		OutputPath    string
		Commit        bool
		NoRestore     bool
		FailOnSkipped bool
		Color         bool
		NoColor       bool
		FullDiff      bool
		NoDiff        bool
		Snapshot      string
		StatsFile     string // External statistics file to apply instead of ANALYZE (PG18+)
	}

	UpdateOptions struct {
		Root        string
		RunFilter   string
		Paths       []string
		Commit      bool
		NoRestore   bool
		Pending     bool
		Interactive bool
		DryRun      bool
		Snapshot    string
	}
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
func Update(opts UpdateOptions) {
	config, err := ReadConfig(opts.Root)
	ignorePatterns := []string{}
	if err == nil {
		ignorePatterns = config.Ignore
	}

	suite := Walk(opts.Root, ignorePatterns)
	suite.SetRunFilter(opts.RunFilter)
	suite.SetPathFilters(opts.Paths)
	config, err = suite.readConfig()
	if err != nil {
		fmt.Print(err.Error())
		os.Exit(3)
	}

	// Load snapshot metadata for baseline tracking
	snapshotsDir := GetSnapshotsDir(opts.Root)
	snapshotMeta, _ := ReadSnapshotMetadata(snapshotsDir)

	// If specific snapshot requested, resolve and use it
	var snapshotOverride string
	var currentSnapshot *SnapshotInfo
	if opts.Snapshot != "" {
		if snapshotMeta == nil {
			fmt.Printf("Error: cannot resolve snapshot %q: no snapshot metadata found\n", opts.Snapshot)
			os.Exit(1)
		}
		info, err := ResolveSnapshot(snapshotMeta, opts.Snapshot)
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			os.Exit(1)
		}
		if !SnapshotExists(info) {
			fmt.Printf("Error: snapshot file not found: %s\n  Tag: %s\n  The snapshot file may have been deleted.\n", info.Path, FormatSnapshotRef(info))
			os.Exit(1)
		}
		snapshotOverride = info.Path
		currentSnapshot = info
		fmt.Printf("Using snapshot: %s (%s)\n", FormatSnapshotRef(info), info.Path)
	} else if snapshotMeta != nil {
		currentSnapshot = snapshotMeta.Current
	}

	maybeRestore(config, opts.Root, opts.NoRestore, snapshotOverride, "")

	// Validate schema hasn't changed since last snapshot build
	if err := ValidateSchemaHash(opts.Root); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	// Validate migrations haven't changed since last snapshot build
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

	updateOpts := createExpectedOptions{
		Commit:      opts.Commit,
		Pending:     opts.Pending,
		Interactive: opts.Interactive,
		DryRun:      opts.DryRun,
		Snapshot:    currentSnapshot,
	}
	if err := suite.createExpectedResults(config.PgUri, updateOpts); err != nil {
		fmt.Print(err.Error())
		os.Exit(12)
	}

	if opts.DryRun {
		return // Don't print success message for dry-run
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

// maybeRestore restores the snapshot if configured and not skipped.
// snapshotOverride allows using a specific snapshot instead of the configured one.
// statsFiles, if provided, are applied instead of running ANALYZE (requires PG18+).
func maybeRestore(cfg config, root string, noRestore bool, snapshotOverride string, statsFile string) {
	if noRestore {
		return
	}

	// Determine snapshot path - use override or configured path
	snapshotPath := snapshotOverride
	if snapshotPath == "" {
		if cfg.Snapshot == nil || cfg.Snapshot.Path == "" {
			return // no snapshot configured
		}
		snapshotPath = GetSnapshotPath(cfg.Snapshot, root)
	}

	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		fmt.Printf("Error: snapshot file not found: %s\n\nRun 'regresql snapshot build' to create a snapshot, or use '--no-restore' to skip\n", snapshotPath)
		os.Exit(1)
	}

	targetDB := ""
	if cfg.Snapshot != nil {
		targetDB = cfg.Snapshot.RestoreDatabase
	}

	fmt.Printf("Restoring snapshot: %s\n", snapshotPath)

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

	// Run ANALYZE or apply external stats after restore
	db, err := OpenDB(cfg.PgUri)
	if err != nil {
		fmt.Printf("Warning: failed to connect: %s\n", err)
	} else {
		defer db.Close()

		if statsFile != "" {
			if err := ApplyStatistics(db, statsFile); err != nil {
				fmt.Printf("Error: %s\n", err)
				os.Exit(1)
			}
			fmt.Printf("Applied statistics: %s\n", statsFile)
		} else {
			if _, err := db.Exec("ANALYZE"); err != nil {
				fmt.Printf("Warning: ANALYZE failed: %s\n", err)
			}
		}
	}

	fmt.Printf("Restored in %.1fs\n\n", time.Since(start).Seconds())
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

	// If specific snapshot requested, resolve and use it
	var snapshotOverride string
	if opts.Snapshot != "" {
		snapshotsDir := GetSnapshotsDir(opts.Root)
		metadata, err := ReadSnapshotMetadata(snapshotsDir)
		if err != nil {
			fmt.Printf("Error: cannot resolve snapshot %q: %s\n", opts.Snapshot, err)
			os.Exit(1)
		}
		info, err := ResolveSnapshot(metadata, opts.Snapshot)
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			os.Exit(1)
		}
		if !SnapshotExists(info) {
			fmt.Printf("Error: snapshot file not found: %s\n  Tag: %s\n  The snapshot file may have been deleted.\n", info.Path, FormatSnapshotRef(info))
			os.Exit(1)
		}
		snapshotOverride = info.Path
		fmt.Printf("Using snapshot: %s (%s)\n", FormatSnapshotRef(info), info.Path)
	}

	maybeRestore(config, opts.Root, opts.NoRestore, snapshotOverride, opts.StatsFile)

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
