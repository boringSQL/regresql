package regresql

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type (
	SnapshotBuildOptions struct {
		OutputPath       string
		Format           SnapshotFormat
		SchemaPath       string
		MigrationsDir    string
		MigrationCommand string
		Fixtures         []string
		Verbose          bool
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

	// Check pg_restore is available for non-plain schema files
	if opts.SchemaPath != "" && DetectSnapshotFormat(opts.SchemaPath) != FormatPlain {
		if err := CheckPgTool("pg_restore", root); err != nil {
			return nil, err
		}
	}

	if len(opts.Fixtures) == 0 && opts.SchemaPath == "" && opts.MigrationsDir == "" && opts.MigrationCommand == "" {
		return nil, fmt.Errorf("no schema, migrations, or fixtures specified for snapshot build")
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

	// Apply schema first if provided
	var schemaHash string
	if opts.SchemaPath != "" {
		if opts.Verbose {
			format := DetectSnapshotFormat(opts.SchemaPath)
			fmt.Printf("Applying schema: %s (format: %s)\n", opts.SchemaPath, format)
		}
		if err := applySchemaFile(tempDB.PgUri, opts.SchemaPath); err != nil {
			return nil, fmt.Errorf("schema %q: %w", opts.SchemaPath, err)
		}
		schemaHash, err = computeSchemaHash(opts.SchemaPath)
		if err != nil {
			return nil, fmt.Errorf("failed to compute schema hash: %w", err)
		}
	}

	// Apply migrations - either from directory or via external command (mutually exclusive)
	var migrationsApplied []string
	var migrationsHash string
	var migrationCommandHash string

	if opts.MigrationsDir != "" {
		migrationFiles, err := discoverMigrations(opts.MigrationsDir)
		if err != nil {
			return nil, fmt.Errorf("failed to discover migrations: %w", err)
		}

		if len(migrationFiles) > 0 {
			if opts.Verbose {
				fmt.Printf("Applying %d migration(s)...\n", len(migrationFiles))
			}
			if err := applyMigrations(db, migrationFiles, opts.Verbose); err != nil {
				return nil, err
			}

			for _, f := range migrationFiles {
				migrationsApplied = append(migrationsApplied, filepath.Base(f))
			}

			migrationsHash, err = computeMigrationsHash(migrationFiles)
			if err != nil {
				return nil, fmt.Errorf("failed to compute migrations hash: %w", err)
			}
		}
	} else if opts.MigrationCommand != "" {
		if err := runMigrationCommand(opts.MigrationCommand, tempDB.PgUri, opts.Verbose); err != nil {
			return nil, err
		}
		migrationCommandHash = computeCommandHash(opts.MigrationCommand)
	}

	var fixturesUsed []string
	if len(opts.Fixtures) > 0 {
		if opts.Verbose {
			fmt.Printf("Applying %d fixture(s)...\n", len(opts.Fixtures))
		}
		fixturesUsed, err = applyFixtures(db, root, opts.Fixtures, opts.Verbose)
		if err != nil {
			return nil, err
		}
	}

	// Capture server context before snapshot
	if opts.Verbose {
		fmt.Printf("Capturing server context...\n")
	}
	serverCtx, err := CaptureServerContext(db)
	if err != nil {
		return nil, fmt.Errorf("failed to capture server context: %w", err)
	}

	// Run ANALYZE to ensure statistics are up to date before pg_dump
	if opts.Verbose {
		fmt.Printf("Running ANALYZE...\n")
	}
	if _, err := db.Exec("ANALYZE"); err != nil {
		return nil, fmt.Errorf("failed to analyze database: %w", err)
	}

	if opts.Verbose {
		fmt.Printf("Capturing snapshot with pg_dump...\n")
	}

	info, err := CaptureSnapshot(tempDB.PgUri, SnapshotOptions{
		OutputPath:     opts.OutputPath,
		Format:         opts.Format,
		WithStatistics: serverCtx.MajorVersion() >= 18,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to capture snapshot: %w", err)
	}

	info.SchemaPath = opts.SchemaPath
	info.SchemaHash = schemaHash
	info.MigrationsDir = opts.MigrationsDir
	info.MigrationsHash = migrationsHash
	info.MigrationsApplied = migrationsApplied
	info.MigrationCommand = opts.MigrationCommand
	info.MigrationCommandHash = migrationCommandHash
	info.FixturesUsed = fixturesUsed
	info.Server = serverCtx

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
	var yamlFixtures []string

	// First pass: execute SQL fixtures immediately, collect YAML fixtures
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
			yamlFixtures = append(yamlFixtures, name)
		}
	}

	// Second pass: apply all YAML fixtures in a single transaction
	// This ensures dependencies are resolved correctly and not re-applied
	if len(yamlFixtures) > 0 {
		if verbose {
			for _, name := range yamlFixtures {
				fmt.Printf("  Applying fixture: %s\n", name)
			}
		}
		if err := applyYAMLFixtures(fm, yamlFixtures); err != nil {
			return nil, err
		}
		applied = append(applied, yamlFixtures...)
	}

	return applied, nil
}

func applyYAMLFixtures(fm *FixtureManager, names []string) error {
	if err := fm.BeginTransaction(); err != nil {
		return err
	}

	_ = fm.IntrospectSchema()

	if err := fm.ApplyFixtures(names); err != nil {
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

func applySchemaFile(pguri, schemaPath string) error {
	format := DetectSnapshotFormat(schemaPath)

	if format == FormatPlain {
		db, err := OpenDB(pguri)
		if err != nil {
			return err
		}
		defer db.Close()
		return execSQLFile(db, schemaPath)
	}

	// Custom or Directory format - use pg_restore --schema-only
	args := []string{
		"--dbname", pguri,
		"--schema-only",
		"--no-owner",
		"--no-acl",
	}
	if format == FormatDirectory {
		args = append(args, "--format=directory")
	}
	args = append(args, schemaPath)

	cmd := exec.Command("pg_restore", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore failed: %w", err)
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

func GetSnapshotSchema(cfg *SnapshotConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.Schema
}

func GetSnapshotMigrations(cfg *SnapshotConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.Migrations
}

func GetSnapshotMigrationCommand(cfg *SnapshotConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.MigrationCommand
}

// runMigrationCommand executes an external migration tool with PGURI env var set
func runMigrationCommand(command, pguri string, verbose bool) error {
	if verbose {
		fmt.Printf("Running migration command: %s\n", command)
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), "PGURI="+pguri, "DATABASE_URL="+pguri)

	output, err := cmd.CombinedOutput()
	if verbose && len(output) > 0 {
		fmt.Printf("%s", output)
	}

	if err != nil {
		// Always show output on error, even if not verbose
		if !verbose && len(output) > 0 {
			return fmt.Errorf("migration command failed: %w\n%s", err, output)
		}
		return fmt.Errorf("migration command failed: %w", err)
	}
	return nil
}

func computeCommandHash(command string) string {
	h := sha256.Sum256([]byte(command))
	return "sha256:" + hex.EncodeToString(h[:])
}

// discoverMigrations finds *.sql files in directory (skips *.down.sql), sorted by name
func discoverMigrations(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".sql") {
			continue
		}
		// Skip .down.sql files (reverse migrations)
		if strings.HasSuffix(lower, ".down.sql") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files) // Lexical sort: 001_init.sql, 002_users.sql, etc.
	return files, nil
}

// applyMigrations executes migration files in order
func applyMigrations(db *sql.DB, files []string, verbose bool) error {
	for _, f := range files {
		if verbose {
			fmt.Printf("  Migration: %s\n", filepath.Base(f))
		}
		if err := execSQLFile(db, f); err != nil {
			return fmt.Errorf("migration %q: %w", filepath.Base(f), err)
		}
	}
	return nil
}

// computeMigrationsHash computes combined hash of all migration files
func computeMigrationsHash(files []string) (string, error) {
	h := sha256.New()
	for _, f := range files {
		// Include filename in hash for ordering sensitivity
		h.Write([]byte(filepath.Base(f)))

		content, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h.Write(content)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
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
