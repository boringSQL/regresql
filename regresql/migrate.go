package regresql

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type (
	MigrateOptions struct {
		Root     string
		Script   string
		Command  string
		KeepTemp bool
		Verbose  bool
		Color    bool
		NoColor  bool
		FullDiff bool
		NoDiff   bool
	}

	MigrateResult struct {
		QueriesRun  int
		Differences int
		Duration    time.Duration
		Diffs       []MigrateDiff
	}

	MigrateDiff struct {
		QueryPath   string
		BindingName string
		BeforeFile  string
		AfterFile   string
		Identical   bool
		Diff        *StructuredDiff
	}
)

// Migrate tests query output changes before/after a migration.
// Returns exit code: 0 = no differences, 1 = differences found or error
func Migrate(opts MigrateOptions) int {
	startTime := time.Now()

	// 1. Read config
	cfg, err := ReadConfig(opts.Root)
	if err != nil {
		fmt.Printf("Error reading config: %s\n", err)
		return 3
	}

	// 2. Restore snapshot (required for migration testing)
	snapshotPath := GetSnapshotPath(cfg.Snapshot, opts.Root)
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		fmt.Printf("Error: snapshot not found: %s\n", snapshotPath)
		fmt.Println("Migration testing requires a snapshot to restore the pre-migration state.")
		fmt.Println("Run 'regresql snapshot build' to create a snapshot first.")
		return 1
	}

	fmt.Printf("Restoring snapshot: %s\n", snapshotPath)
	restoreStart := time.Now()
	restoreOpts := RestoreOptions{
		InputPath:      snapshotPath,
		Clean:          true,
		TargetDatabase: cfg.Snapshot.RestoreDatabase,
	}
	if err := RestoreSnapshot(cfg.PgUri, restoreOpts); err != nil {
		fmt.Printf("Error: failed to restore snapshot: %s\n", err)
		return 1
	}
	fmt.Printf("Restored in %.1fs\n\n", time.Since(restoreStart).Seconds())

	// 3. Create temp directories for before/after results
	tempDir, err := os.MkdirTemp("", "regresql-migrate-")
	if err != nil {
		fmt.Printf("Error: failed to create temp directory: %s\n", err)
		return 1
	}
	defer func() {
		if opts.KeepTemp {
			fmt.Printf("\nTemp directory preserved: %s\n", tempDir)
		} else {
			os.RemoveAll(tempDir)
		}
	}()

	beforeDir := filepath.Join(tempDir, "before")
	afterDir := filepath.Join(tempDir, "after")
	if err := os.MkdirAll(beforeDir, 0755); err != nil {
		fmt.Printf("Error: failed to create before directory: %s\n", err)
		return 1
	}
	if err := os.MkdirAll(afterDir, 0755); err != nil {
		fmt.Printf("Error: failed to create after directory: %s\n", err)
		return 1
	}

	// 4. Discover queries
	ignorePatterns := cfg.Ignore
	suite := Walk(opts.Root, ignorePatterns)

	// 5. Execute all queries -> save to beforeDir
	fmt.Println("Capturing query results BEFORE migration...")
	beforeCount, err := suite.executeAllQueries(cfg.PgUri, beforeDir, opts.Verbose)
	if err != nil {
		fmt.Printf("Error executing queries: %s\n", err)
		return 1
	}
	fmt.Printf("  %d queries executed\n\n", beforeCount)

	// 6. Apply migration
	fmt.Println("Applying migration...")
	if err := applyMigration(cfg.PgUri, opts); err != nil {
		fmt.Printf("Error applying migration: %s\n", err)
		return 1
	}
	fmt.Println()

	// 7. Execute all queries -> save to afterDir
	fmt.Println("Capturing query results AFTER migration...")
	afterCount, err := suite.executeAllQueries(cfg.PgUri, afterDir, opts.Verbose)
	if err != nil {
		fmt.Printf("Error executing queries: %s\n", err)
		return 1
	}
	fmt.Printf("  %d queries executed\n\n", afterCount)

	// 8. Compare before/after directories
	result := compareBeforeAfter(beforeDir, afterDir)
	result.QueriesRun = beforeCount
	result.Duration = time.Since(startTime)

	// 9. Report results
	reportMigrateResults(result, opts)

	// 10. Return exit code
	if result.Differences > 0 {
		return 1
	}
	return 0
}

// applyMigration applies the migration using either a script file or external command
func applyMigration(pguri string, opts MigrateOptions) error {
	if opts.Script != "" {
		fmt.Printf("  %s\n", opts.Script)
		db, err := OpenDB(pguri)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer db.Close()
		return execSQLFile(db, opts.Script)
	}

	if opts.Command != "" {
		fmt.Printf("  Command: %s\n", opts.Command)
		return runMigrationCommand(opts.Command, pguri, opts.Verbose)
	}

	return fmt.Errorf("no migration script or command specified")
}

// compareBeforeAfter compares all result files in before and after directories
func compareBeforeAfter(beforeDir, afterDir string) *MigrateResult {
	result := &MigrateResult{
		Diffs: []MigrateDiff{},
	}

	// Walk through before directory and find all JSON files
	err := filepath.Walk(beforeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		relPath, _ := filepath.Rel(beforeDir, path)
		afterPath := filepath.Join(afterDir, relPath)

		// Check if after file exists
		if _, err := os.Stat(afterPath); os.IsNotExist(err) {
			// Query failed after migration
			result.Diffs = append(result.Diffs, MigrateDiff{
				QueryPath:  relPath,
				BeforeFile: path,
				AfterFile:  "",
				Identical:  false,
			})
			result.Differences++
			return nil
		}

		// Load and compare result sets
		beforeRS, err := LoadResultSet(path)
		if err != nil {
			return nil // Skip files that can't be loaded
		}
		afterRS, err := LoadResultSet(afterPath)
		if err != nil {
			return nil
		}

		diff := CompareResultSets(beforeRS, afterRS, nil)

		migrateDiff := MigrateDiff{
			QueryPath:  relPath,
			BeforeFile: path,
			AfterFile:  afterPath,
			Identical:  diff.Identical,
			Diff:       diff,
		}

		if !migrateDiff.Identical {
			result.Differences++
		}
		result.Diffs = append(result.Diffs, migrateDiff)

		return nil
	})

	if err != nil {
		fmt.Printf("Warning: error walking before directory: %s\n", err)
	}

	return result
}

// reportMigrateResults prints the migration test results
func reportMigrateResults(result *MigrateResult, opts MigrateOptions) {
	useColor := shouldUseColor(opts.Color, opts.NoColor)

	// Progress indicator
	fmt.Print("MIGRATION IMPACT:\n  ")
	for _, d := range result.Diffs {
		if d.Identical {
			if useColor {
				fmt.Print("\033[32m.\033[0m ")
			} else {
				fmt.Print(". ")
			}
		} else {
			if useColor {
				fmt.Print("\033[31mF\033[0m ")
			} else {
				fmt.Print("F ")
			}
		}
	}
	fmt.Println()
	fmt.Println()

	// Summary
	unchanged := len(result.Diffs) - result.Differences
	fmt.Println("RESULTS:")
	if useColor {
		fmt.Printf("  \033[32m%d unchanged\033[0m\n", unchanged)
		if result.Differences > 0 {
			fmt.Printf("  \033[31m%d affected\033[0m\n", result.Differences)
		}
	} else {
		fmt.Printf("  %d unchanged\n", unchanged)
		if result.Differences > 0 {
			fmt.Printf("  %d affected\n", result.Differences)
		}
	}
	fmt.Println()

	// Show differences if any (unless --no-diff)
	if result.Differences > 0 && !opts.NoDiff {
		fmt.Println("DIFFERENCES:")
		for _, d := range result.Diffs {
			if d.Identical {
				continue
			}

			if useColor {
				fmt.Printf("  \033[31m%s\033[0m\n", d.QueryPath)
			} else {
				fmt.Printf("  %s\n", d.QueryPath)
			}

			if d.Diff != nil {
				printMigrateDiff(d.Diff, opts.FullDiff, useColor)
			} else if d.AfterFile == "" {
				fmt.Println("    Query failed after migration")
			}
			fmt.Println()
		}
	}

	fmt.Printf("Duration: %.1fs\n", result.Duration.Seconds())

	if result.Differences > 0 {
		fmt.Println("\nTip: If these changes are expected, run 'regresql update' to update baselines")
	}
}

// printMigrateDiff prints a single diff in a readable format
func printMigrateDiff(diff *StructuredDiff, fullDiff, useColor bool) {
	// Row count change
	if diff.ExpectedRows != diff.ActualRows {
		fmt.Printf("    Rows: %d -> %d\n", diff.ExpectedRows, diff.ActualRows)
	}

	// Diff type
	fmt.Printf("    Type: %s\n", diff.Type)

	// Added rows
	if diff.AddedRows > 0 {
		if useColor {
			fmt.Printf("    \033[32m+ Added rows: %d\033[0m\n", diff.AddedRows)
		} else {
			fmt.Printf("    + Added rows: %d\n", diff.AddedRows)
		}
		// Show samples
		maxShow := 3
		if fullDiff {
			maxShow = len(diff.AddedSamples)
		}
		for i, sample := range diff.AddedSamples {
			if i >= maxShow {
				remaining := len(diff.AddedSamples) - maxShow
				fmt.Printf("      ... and %d more (use --diff to see all)\n", remaining)
				break
			}
			fmt.Printf("      %v\n", sample)
		}
	}

	// Removed rows
	if diff.RemovedRows > 0 {
		if useColor {
			fmt.Printf("    \033[31m- Removed rows: %d\033[0m\n", diff.RemovedRows)
		} else {
			fmt.Printf("    - Removed rows: %d\n", diff.RemovedRows)
		}
		// Show samples
		maxShow := 3
		if fullDiff {
			maxShow = len(diff.RemovedSamples)
		}
		for i, sample := range diff.RemovedSamples {
			if i >= maxShow {
				remaining := len(diff.RemovedSamples) - maxShow
				fmt.Printf("      ... and %d more (use --diff to see all)\n", remaining)
				break
			}
			fmt.Printf("      %v\n", sample)
		}
	}

	// Modified rows
	if diff.ModifiedRows > 0 {
		fmt.Printf("    Modified rows: %d\n", diff.ModifiedRows)
		maxShow := 3
		if fullDiff {
			maxShow = len(diff.ModifiedSamples)
		}
		for i, sample := range diff.ModifiedSamples {
			if i >= maxShow {
				remaining := len(diff.ModifiedSamples) - maxShow
				fmt.Printf("      ... and %d more (use --diff to see all)\n", remaining)
				break
			}
			fmt.Printf("      Expected: %v\n", sample.ExpectedRow)
			fmt.Printf("      Actual:   %v\n", sample.ActualRow)
		}
	}
}

// shouldUseColor determines if colored output should be used
func shouldUseColor(forceColor, noColor bool) bool {
	if noColor {
		return false
	}
	if forceColor {
		return true
	}
	// Check if stdout is a terminal
	stat, _ := os.Stdout.Stat()
	return (stat.Mode() & os.ModeCharDevice) != 0
}
