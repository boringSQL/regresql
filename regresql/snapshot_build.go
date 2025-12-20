package regresql

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type (
	SnapshotBuildOptions struct {
		OutputPath string
		Format     SnapshotFormat
		Fixtures   []string
		Verbose    bool
	}

	snapshotBuildResult struct {
		Info         *SnapshotInfo
		FixturesUsed []string
		Duration     time.Duration
	}
)

func BuildSnapshot(basePgUri string, root string, opts SnapshotBuildOptions) (*snapshotBuildResult, error) {
	startTime := time.Now()

	if err := CheckPgTool("pg_dump", root); err != nil {
		return nil, err
	}

	if len(opts.Fixtures) == 0 {
		return nil, fmt.Errorf("no fixtures specified for snapshot build")
	}

	if opts.Verbose {
		fmt.Printf("Creating temporary database...\n")
	}

	tempDB, err := CreateTempDB(TempDBOptions{BasePgUri: basePgUri})
	if err != nil {
		return nil, fmt.Errorf("failed to create temp database: %w", err)
	}
	defer func() {
		if opts.Verbose {
			fmt.Printf("Dropping temporary database %s...\n", tempDB.Name)
		}
		if err := tempDB.Drop(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to drop temp database: %v\n", err)
		}
	}()

	if opts.Verbose {
		fmt.Printf("Temporary database created: %s\n", tempDB.Name)
	}

	db, err := OpenDB(tempDB.PgUri)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to temp database: %w", err)
	}
	defer db.Close()

	if opts.Verbose {
		fmt.Printf("Applying %d fixture(s)...\n", len(opts.Fixtures))
	}

	fixturesUsed, err := applyFixtures(db, root, opts.Fixtures, opts.Verbose)
	if err != nil {
		return nil, err
	}

	if opts.Verbose {
		fmt.Printf("Fixtures applied successfully\n")
		fmt.Printf("Capturing snapshot with pg_dump...\n")
	}

	info, err := CaptureSnapshot(tempDB.PgUri, SnapshotOptions{
		OutputPath: opts.OutputPath,
		Format:     opts.Format,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to capture snapshot: %w", err)
	}

	info.FixturesUsed = fixturesUsed

	return &snapshotBuildResult{
		Info:         info,
		FixturesUsed: fixturesUsed,
		Duration:     time.Since(startTime),
	}, nil
}

// applyFixtures processes fixtures in order. SQL files are executed directly,
// YAML fixtures go through FixtureManager.
func applyFixtures(db *sql.DB, root string, fixtures []string, verbose bool) ([]string, error) {
	fm, err := NewFixtureManager(root, db)
	if err != nil {
		return nil, fmt.Errorf("failed to create fixture manager: %w", err)
	}

	var applied []string

	for _, f := range fixtures {
		if isSQLFixture(f) {
			if verbose {
				fmt.Printf("  Executing SQL: %s\n", f)
			}
			if err := execSQLFile(db, filepath.Join(root, f)); err != nil {
				return nil, fmt.Errorf("fixture %q: %w", f, err)
			}
			applied = append(applied, f)
		} else {
			name := trimYAMLExt(f)
			if verbose {
				fmt.Printf("  Applying fixture: %s\n", name)
			}
			if err := applyYAMLFixture(fm, name); err != nil {
				return nil, fmt.Errorf("fixture %q: %w", name, err)
			}
			applied = append(applied, name)
		}
	}

	return applied, nil
}

func applyYAMLFixture(fm *FixtureManager, name string) error {
	if err := fm.BeginTransaction(); err != nil {
		return err
	}

	_ = fm.IntrospectSchema()

	if err := fm.ApplyFixtures([]string{name}); err != nil {
		fm.Rollback()
		return err
	}

	return fm.Commit()
}

func execSQLFile(db *sql.DB, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if _, err := db.Exec(string(content)); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	return nil
}

func isSQLFixture(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".sql")
}

func trimYAMLExt(name string) string {
	name = strings.TrimSuffix(name, ".yaml")
	name = strings.TrimSuffix(name, ".yml")
	return name
}

func GetSnapshotFixtures(cfg *SnapshotConfig) []string {
	if cfg == nil {
		return nil
	}
	return cfg.Fixtures
}

// FixturesExist validates that all fixture files exist before build.
func FixturesExist(root string, fixtures []string) error {
	for _, f := range fixtures {
		if isSQLFixture(f) {
			if err := checkFile(filepath.Join(root, f)); err != nil {
				return fmt.Errorf("SQL fixture %q: %w", f, err)
			}
		} else {
			name := trimYAMLExt(f)
			if err := checkYAMLFixture(root, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return nil
}

func checkYAMLFixture(root, name string) error {
	dir := filepath.Join(root, "regresql", "fixtures")
	for _, ext := range []string{".yaml", ".yml"} {
		if _, err := os.Stat(filepath.Join(dir, name+ext)); err == nil {
			return nil
		}
	}
	return fmt.Errorf("fixture %q not found in %s", name, dir)
}
