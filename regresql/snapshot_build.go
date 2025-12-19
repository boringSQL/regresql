package regresql

import (
	"fmt"
	"os"
	"path/filepath"
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
		if dropErr := tempDB.Drop(); dropErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to drop temp database: %v\n", dropErr)
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

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping temp database: %w", err)
	}

	if opts.Verbose {
		fmt.Printf("Connected to temporary database\n")
		fmt.Printf("Applying %d fixture(s)...\n", len(opts.Fixtures))
	}

	fm, err := NewFixtureManager(root, db)
	if err != nil {
		return nil, fmt.Errorf("failed to create fixture manager: %w", err)
	}

	fixtures, err := fm.ResolveDependencies(opts.Fixtures)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve fixture dependencies: %w", err)
	}

	if opts.Verbose {
		for _, f := range fixtures {
			fmt.Printf("  Will apply: %s\n", f.Name)
		}
	}

	if err := fm.BeginTransaction(); err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	_ = fm.IntrospectSchema()

	if err := fm.ApplyFixtures(opts.Fixtures); err != nil {
		fm.Rollback()
		return nil, fmt.Errorf("failed to apply fixtures: %w", err)
	}

	if err := fm.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit fixtures: %w", err)
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

	fixturesUsed := make([]string, len(fixtures))
	for i, f := range fixtures {
		fixturesUsed[i] = f.Name
	}
	info.FixturesUsed = fixturesUsed

	return &snapshotBuildResult{
		Info:         info,
		FixturesUsed: fixturesUsed,
		Duration:     time.Since(startTime),
	}, nil
}

func GetSnapshotFixtures(cfg *SnapshotConfig) []string {
	if cfg == nil {
		return nil
	}
	return cfg.Fixtures
}

func FixturesExist(root string, fixtureNames []string) error {
	fixturesDir := filepath.Join(root, "regresql", "fixtures")

	for _, name := range fixtureNames {
		yamlPath := filepath.Join(fixturesDir, name+".yaml")
		ymlPath := filepath.Join(fixturesDir, name+".yml")

		if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
			if _, err := os.Stat(ymlPath); os.IsNotExist(err) {
				return fmt.Errorf("fixture %q not found at %s or %s", name, yamlPath, ymlPath)
			}
		}
	}

	return nil
}
