package regresql

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		Stats         string // Stats profile name, YAML path, or SQL path
		Verbose       bool
		Strict        bool
		Generic       bool
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

That means creating the ./regresql/ directory and the regress.yaml config.
Use 'regresql add' to add individual queries to the test suite.
*/
func Init(root string, pguri string) {
	if err := TestConnectionString(pguri); err != nil {
		fmt.Println(err)
		fmt.Println("Check the connection string and that the server is running; nothing was written.")
		os.Exit(2)
	}

	suite := Walk(root, []string{})

	suite.createRegressDir()
	suite.setupConfig(pguri)

	fmt.Println("")
	fmt.Println("Initialized RegreSQL in ./regresql/")
	fmt.Println("")
	fmt.Println(`Use 'regresql add <path/to/query.sql>' to add queries to the test suite.`)
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
		fmt.Printf("Error: %s\n", err)
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

	sf := useStatsFile(config, opts.Root, "", os.Stdout)
	sf = dropUnappliedStats(config, opts.NoRestore, snapshotOverride, sf)
	maybeRestore(config, opts.Root, opts.NoRestore, snapshotOverride, sf)

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

// progress receives status lines. Test redirects it to stderr when stdout
// carries a machine-readable report.
var progress io.Writer = os.Stdout

func progressWriter(format, outputPath string) io.Writer {
	if format != "" && format != "console" && outputPath == "" {
		return os.Stderr
	}
	return os.Stdout
}

// maybeRestore restores the snapshot if configured and not skipped.
// snapshotOverride allows using a specific snapshot instead of the configured one.
// sf is the resolved SQL stats file to apply instead of ANALYZE. Empty means ANALYZE.
func maybeRestore(cfg config, root string, noRestore bool, snapshotOverride string, sf string) {
	if !restoreRuns(cfg, noRestore, snapshotOverride) {
		return
	}

	snapshotPath := snapshotOverride
	if snapshotPath == "" {
		snapshotPath = GetSnapshotPath(cfg.Snapshot, root)
	}

	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		fmt.Fprintf(progress, "Error: snapshot file not found: %s\n\nRun 'regresql snapshot build' to create a snapshot, or use '--no-restore' to skip\n", snapshotPath)
		os.Exit(1)
	}

	targetDB := ""
	if cfg.Snapshot != nil {
		targetDB = cfg.Snapshot.RestoreDatabase
	}

	fmt.Fprintf(progress, "Restoring snapshot: %s\n", snapshotPath)

	start := time.Now()
	opts := RestoreOptions{
		InputPath:      snapshotPath,
		Clean:          true,
		TargetDatabase: targetDB,
	}
	if err := RestoreSnapshot(cfg.PgUri, opts); err != nil {
		fmt.Fprintf(progress, "Error: failed to restore snapshot: %s\n", err)
		os.Exit(1)
	}

	// Run ANALYZE or apply external stats after restore
	db, err := openSessionDB(cfg.PgUri)
	if err != nil {
		fmt.Fprintf(progress, "Warning: failed to connect: %s\n", err)
	} else {
		defer db.Close()

		if sf != "" {
			if err := ApplyStatistics(db, sf); err != nil {
				fmt.Fprintf(progress, "Error: %s\n", err)
				os.Exit(15)
			}
			fmt.Fprintf(progress, "Applied statistics: %s\n", sf)
		} else {
			if _, err := db.Exec("ANALYZE"); err != nil {
				fmt.Fprintf(progress, "Warning: ANALYZE failed: %s\n", err)
			}
		}
	}

	fmt.Fprintf(progress, "Restored in %.1fs\n\n", time.Since(start).Seconds())
}

// restoreRuns reports whether maybeRestore will restore a snapshot.
func restoreRuns(cfg config, noRestore bool, snapshotOverride string) bool {
	if noRestore {
		return false
	}
	return snapshotOverride != "" || (cfg.Snapshot != nil && cfg.Snapshot.Path != "")
}

// dropUnappliedStats clears the stats file when no restore will apply it and
// the database does not already carry injected stats.
func dropUnappliedStats(cfg config, noRestore bool, snapshotOverride, sf string) string {
	if sf == "" || restoreRuns(cfg, noRestore, snapshotOverride) {
		return sf
	}
	present, err := StatsPresent(cfg.PgUri)
	if err != nil || present {
		return sf
	}
	fmt.Fprintf(progress, "Warning: stats file %s is not applied (no snapshot restore) and the database has no injected stats; running in buffers mode\n", sf)
	injectedStats = false
	return ""
}

// useStatsFile resolves the stats file (flag > config default) and records
// whether injection is in use.
func useStatsFile(cfg config, root, flag string, warn io.Writer) string {
	sf := flag
	if sf == "" && cfg.Stats != nil && cfg.Stats.Default != "" {
		sf = cfg.Stats.Default
		if !filepath.IsAbs(sf) {
			sf = filepath.Join(root, "regresql", sf)
		}
	}
	if sf != "" {
		if _, err := os.Stat(sf); os.IsNotExist(err) {
			fmt.Fprintf(warn, "Warning: stats file not found: %s; not injecting\n", sf)
			sf = ""
		}
	}
	injectedStats = sf != ""
	return sf
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

	fmt.Fprintf(progress, "%s\n\n", warning)
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
//	10 - severity violation (error-level plan warning/regression, or any
//	     warning-level finding when --strict)
//	13 - query execution error
//	14 - invalid formatter
//	15 - environment not fit to run (plan choice mode without pg_regresql)
func Test(opts TestOptions) {
	progress = progressWriter(opts.FormatName, opts.OutputPath)
	defer func() { progress = os.Stdout }()

	config, err := ReadConfig(opts.Root)
	ignorePatterns := []string{}
	if err == nil {
		ignorePatterns = config.Ignore
	}

	suite := Walk(opts.Root, ignorePatterns)
	suite.SetRunFilter(opts.RunFilter)
	config, err = suite.readConfig()
	if err != nil {
		fmt.Fprintf(progress, "Error: %s\n", err)
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
			fmt.Fprintf(progress, "Error: cannot resolve snapshot %q: %s\n", opts.Snapshot, err)
			os.Exit(1)
		}
		info, err := ResolveSnapshot(metadata, opts.Snapshot)
		if err != nil {
			fmt.Fprintf(progress, "Error: %s\n", err)
			os.Exit(1)
		}
		if !SnapshotExists(info) {
			fmt.Fprintf(progress, "Error: snapshot file not found: %s\n  Tag: %s\n  The snapshot file may have been deleted.\n", info.Path, FormatSnapshotRef(info))
			os.Exit(1)
		}
		snapshotOverride = info.Path
		fmt.Fprintf(progress, "Using snapshot: %s (%s)\n", FormatSnapshotRef(info), info.Path)
	}

	formatName := opts.FormatName
	if formatName == "" {
		formatName = "console"
	}

	sf := useStatsFile(config, opts.Root, opts.Stats, progress)
	sf = dropUnappliedStats(config, opts.NoRestore, snapshotOverride, sf)
	env := CollectEnv(config.PgUri, sf, config, opts.Root, false)
	headerOut := io.Writer(os.Stderr)
	if formatName == "console" {
		headerOut = os.Stdout
	}
	if env.AbortRun() {
		writeEnvHeader(headerOut, env)
		fmt.Fprintln(headerOut, env.AbortMessage())
		os.Exit(15)
	}
	if opts.Generic && env.Mode != ModePlanChoice {
		fmt.Fprintln(headerOut, "Error: --generic needs plan choice mode, but no stats file is in use.")
		fmt.Fprintln(headerOut, "Pass --stats <file> (or set stats.default). It is applied on snapshot restore (snapshot.path, without --no-restore), or used as is when the database already carries injected stats.")
		os.Exit(3)
	}
	if formatName != "console" {
		writeEnvHeader(os.Stderr, env)
	}
	if env.PGMajor == envMismatch {
		fmt.Fprintln(headerOut, "Warning: stats file was dumped from a different PostgreSQL major than the server")
	}

	maybeRestore(config, opts.Root, opts.NoRestore, snapshotOverride, sf)

	// Validate schema hasn't changed since last snapshot build
	if err := ValidateSchemaHash(opts.Root); err != nil {
		fmt.Fprintf(progress, "Error: %s\n", err)
		os.Exit(1)
	}

	// migrations haven't changed since last snapshot build?:
	if err := ValidateMigrationsHash(opts.Root); err != nil {
		fmt.Fprintf(progress, "Error: %s\n", err)
		os.Exit(1)
	}

	// Validate migration command hasn't changed since last snapshot build
	if err := ValidateMigrationCommandHash(opts.Root); err != nil {
		fmt.Fprintf(progress, "Error: %s\n", err)
		os.Exit(1)
	}

	if err := TestConnectionString(config.PgUri); err != nil {
		fmt.Fprint(progress, err.Error())
		os.Exit(2)
	}

	// Validate server settings match snapshot (warn, strict, or ignore)
	if err := validateServerSettings(config, opts.Root); err != nil {
		fmt.Fprintf(progress, "Error: %s\n", err)
		os.Exit(1)
	}

	formatter, err := GetFormatter(formatName)
	if err != nil {
		fmt.Fprintf(progress, "Error: %s\n", err.Error())
		os.Exit(14)
	}

	// Configure console formatter options
	if cf, ok := formatter.(*ConsoleFormatter); ok {
		cf.SetOptions(ConsoleOptions{
			Color:    opts.Color,
			NoColor:  opts.NoColor,
			FullDiff: opts.FullDiff,
			NoDiff:   opts.NoDiff,
			Verbose:  opts.Verbose,
		})
	}

	summary, err := suite.testQueries(config.PgUri, formatter, opts.OutputPath, opts.Commit, opts.Generic, env)
	if err != nil {
		fmt.Fprint(progress, err.Error())
		switch {
		case errors.Is(err, errGenericVersion):
			os.Exit(3)
		case errors.Is(err, errGenericNoExtension):
			os.Exit(15)
		}
		os.Exit(13)
	}
	if summary.Failed > 0 {
		os.Exit(1)
	}
	if opts.FailOnSkipped && summary.Skipped > 0 {
		os.Exit(2)
	}
	if hasSeverityViolation(summary.Results, opts.Strict) {
		os.Exit(10)
	}
}

func hasSeverityViolation(results []TestResult, strict bool) bool {
	match := func(sev string) bool {
		if sev == "error" {
			return true
		}
		return strict && sev == "warning"
	}
	for _, r := range results {
		for _, w := range r.PlanWarnings {
			if match(w.Severity) {
				return true
			}
		}
		for _, reg := range r.PlanRegressions {
			if match(reg.Severity) {
				return true
			}
		}
	}
	return false
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
