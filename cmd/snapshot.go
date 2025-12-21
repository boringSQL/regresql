package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	snapshotCwd             string
	snapshotOutput          string
	snapshotOutputDir       string
	snapshotFormat          string
	snapshotSchemaOnly      bool
	snapshotSection         string
	snapshotSections        bool
	snapshotInput           string
	snapshotClean           bool
	snapshotBuildFixtures   []string
	snapshotBuildSchema     string
	snapshotBuildMigrations string
	snapshotBuildVerbose    bool

	snapshotCmd = &cobra.Command{
		Use:   "snapshot",
		Short: "Manage database snapshots",
		Long:  `Manage database snapshots for reproducible testing.`,
	}

	snapshotCaptureCmd = &cobra.Command{
		Use:   "capture [flags]",
		Short: "Capture current database state as a snapshot",
		Long: `Capture the current database state using pg_dump.

Examples:
  regresql snapshot capture
  regresql snapshot capture --output snapshots/mydata.dump
  regresql snapshot capture --schema-only
  regresql snapshot capture --format plain --output snapshots/schema.sql
  regresql snapshot capture --section pre-data --output snapshots/pre-data.sql
  regresql snapshot capture --sections --output-dir snapshots/`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(snapshotCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			var err error
			if snapshotSections {
				err = runSnapshotCaptureSections()
			} else {
				err = runSnapshotCapture()
			}
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}

	snapshotRestoreCmd = &cobra.Command{
		Use:   "restore [flags]",
		Short: "Restore database from a snapshot",
		Long: `Restore the database state from a previously captured snapshot.

Examples:
  regresql snapshot restore
  regresql snapshot restore --from snapshots/mydata.dump
  regresql snapshot restore --clean`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(snapshotCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			if err := runSnapshotRestore(); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}

	snapshotBuildCmd = &cobra.Command{
		Use:   "build [flags]",
		Short: "Build snapshot from fixtures",
		Long: `Build a reproducible database snapshot from fixtures.

Examples:
  regresql snapshot build
  regresql snapshot build --fixtures users,products,orders
  regresql snapshot build --schema schema.sql --fixtures seed_data
  regresql snapshot build --output snapshots/test_data.dump --verbose`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(snapshotCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			if err := runSnapshotBuild(); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}

	snapshotInfoCmd = &cobra.Command{
		Use:   "info",
		Short: "Display snapshot metadata",
		Long: `Display metadata about the current snapshot.

Shows the snapshot path, hash, size, creation time, and fixtures used.

Examples:
  regresql snapshot info`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(snapshotCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			if err := runSnapshotInfo(); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}
)

func init() {
	RootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotCaptureCmd)
	snapshotCmd.AddCommand(snapshotRestoreCmd)
	snapshotCmd.AddCommand(snapshotBuildCmd)
	snapshotCmd.AddCommand(snapshotInfoCmd)

	snapshotCmd.PersistentFlags().StringVarP(&snapshotCwd, "cwd", "C", ".", "Change to directory")

	snapshotCaptureCmd.Flags().StringVarP(&snapshotOutput, "output", "o", "", "Output file path")
	snapshotCaptureCmd.Flags().StringVar(&snapshotOutputDir, "output-dir", "", "Output directory for sectioned capture")
	snapshotCaptureCmd.Flags().StringVarP(&snapshotFormat, "format", "f", "", "Dump format: custom, plain, or directory")
	snapshotCaptureCmd.Flags().BoolVar(&snapshotSchemaOnly, "schema-only", false, "Dump only schema, no data")
	snapshotCaptureCmd.Flags().StringVar(&snapshotSection, "section", "", "Dump specific section: pre-data, data, or post-data")
	snapshotCaptureCmd.Flags().BoolVar(&snapshotSections, "sections", false, "Capture all sections to separate SQL files")

	snapshotRestoreCmd.Flags().StringVar(&snapshotInput, "from", "", "Input file path")
	snapshotRestoreCmd.Flags().StringVarP(&snapshotFormat, "format", "f", "", "Snapshot format: custom, plain, or directory")
	snapshotRestoreCmd.Flags().BoolVar(&snapshotClean, "clean", false, "Drop existing objects before restore")

	snapshotBuildCmd.Flags().StringVarP(&snapshotOutput, "output", "o", "", "Output file path")
	snapshotBuildCmd.Flags().StringVarP(&snapshotFormat, "format", "f", "", "Dump format: custom, plain, or directory")
	snapshotBuildCmd.Flags().StringVar(&snapshotBuildSchema, "schema", "", "Schema file to apply before migrations")
	snapshotBuildCmd.Flags().StringVar(&snapshotBuildMigrations, "migrations", "", "Directory of SQL migrations to apply")
	snapshotBuildCmd.Flags().StringSliceVar(&snapshotBuildFixtures, "fixtures", nil, "Fixture names to apply")
	snapshotBuildCmd.Flags().BoolVarP(&snapshotBuildVerbose, "verbose", "v", false, "Print detailed progress")
}

func validateSnapshotPrereqs(pguri string) error {
	if pguri == "" {
		return fmt.Errorf("pguri not configured in regress.yaml")
	}
	if err := regresql.TestConnectionString(pguri); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	if err := regresql.CheckPgTool("pg_dump", snapshotCwd); err != nil {
		return err
	}
	return nil
}

func runSnapshotCapture() error {
	cfg, err := regresql.ReadConfig(snapshotCwd)
	if err != nil {
		return fmt.Errorf("failed to read config: %w (have you run 'regresql init'?)", err)
	}
	if err := validateSnapshotPrereqs(cfg.PgUri); err != nil {
		return err
	}

	outputPath := snapshotOutput
	if outputPath == "" {
		outputPath = regresql.GetSnapshotPath(cfg.Snapshot, snapshotCwd)
	} else if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(snapshotCwd, outputPath)
	}

	var format regresql.SnapshotFormat
	if snapshotFormat != "" {
		format = regresql.SnapshotFormat(snapshotFormat)
	} else {
		format = regresql.GetSnapshotFormat(cfg.Snapshot)
	}

	opts := regresql.SnapshotOptions{
		OutputPath: outputPath,
		Format:     format,
		SchemaOnly: snapshotSchemaOnly,
		Section:    snapshotSection,
	}

	fmt.Printf("Capturing database snapshot...\n")
	fmt.Printf("  Database: %s\n", maskConnectionString(cfg.PgUri))
	fmt.Printf("  Output:   %s\n", outputPath)
	fmt.Printf("  Format:   %s\n", format)
	if snapshotSchemaOnly {
		fmt.Printf("  Mode:     schema-only\n")
	}
	if snapshotSection != "" {
		fmt.Printf("  Section:  %s\n", snapshotSection)
	}
	fmt.Println()

	info, err := regresql.CaptureSnapshot(cfg.PgUri, opts)
	if err != nil {
		return err
	}

	snapshotsDir := filepath.Dir(outputPath)
	if err := regresql.WriteSnapshotMetadata(snapshotsDir, info); err != nil {
		fmt.Printf("Warning: failed to write snapshot metadata: %s\n", err)
	}

	fmt.Printf("Snapshot captured successfully.\n")
	fmt.Printf("  Size: %s\n", regresql.FormatBytes(info.SizeBytes))
	fmt.Printf("  Hash: %s\n", info.Hash)
	fmt.Printf("  Time: %s\n", info.Created.Format("2006-01-02 15:04:05 UTC"))

	return nil
}

func runSnapshotCaptureSections() error {
	cfg, err := regresql.ReadConfig(snapshotCwd)
	if err != nil {
		return fmt.Errorf("failed to read config: %w (have you run 'regresql init'?)", err)
	}
	if err := validateSnapshotPrereqs(cfg.PgUri); err != nil {
		return err
	}

	outputDir := snapshotOutputDir
	if outputDir == "" {
		outputDir = regresql.GetSnapshotsDir(snapshotCwd)
	} else if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(snapshotCwd, outputDir)
	}

	fmt.Printf("Capturing database sections...\n")
	fmt.Printf("  Database:   %s\n", maskConnectionString(cfg.PgUri))
	fmt.Printf("  Output dir: %s\n", outputDir)
	fmt.Printf("  Sections:   pre-data, data, post-data\n")
	fmt.Println()

	result, err := regresql.CaptureSections(cfg.PgUri, regresql.SectionsOptions{
		OutputDir: outputDir,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Sections captured successfully.\n")
	fmt.Printf("  Time: %s\n", result.Created.Format("2006-01-02 15:04:05 UTC"))
	fmt.Println()

	var totalSize int64
	for _, s := range result.Sections {
		fmt.Printf("  %s.sql\n", s.Section)
		fmt.Printf("    Size: %s\n", regresql.FormatBytes(s.SizeBytes))
		fmt.Printf("    Hash: %s\n", s.Hash)
		totalSize += s.SizeBytes
	}
	fmt.Println()
	fmt.Printf("  Total: %s\n", regresql.FormatBytes(totalSize))

	return nil
}

func maskConnectionString(pguri string) string {
	masked := pguri
	if idx := findPasswordEnd(pguri); idx > 0 {
		start := findPasswordStart(pguri)
		if start > 0 && start < idx {
			masked = pguri[:start] + "****" + pguri[idx:]
		}
	}
	return masked
}

func findPasswordStart(s string) int {
	idx := 0
	for i := 0; i < len(s)-2; i++ {
		if s[i:i+3] == "://" {
			idx = i + 3
			break
		}
	}
	if idx == 0 {
		return -1
	}
	for i := idx; i < len(s); i++ {
		if s[i] == ':' {
			return i + 1
		}
		if s[i] == '@' {
			return -1
		}
	}
	return -1
}

func findPasswordEnd(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '@' {
			return i
		}
	}
	return -1
}

func runSnapshotRestore() error {
	cfg, err := regresql.ReadConfig(snapshotCwd)
	if err != nil {
		return fmt.Errorf("failed to read config: %w (have you run 'regresql init'?)", err)
	}

	if cfg.PgUri == "" {
		return fmt.Errorf("pguri not configured in regress.yaml")
	}

	if err := regresql.TestConnectionString(cfg.PgUri); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	inputPath := snapshotInput
	if inputPath == "" {
		inputPath = regresql.GetSnapshotPath(cfg.Snapshot, snapshotCwd)
	} else if !filepath.IsAbs(inputPath) {
		inputPath = filepath.Join(snapshotCwd, inputPath)
	}

	var format regresql.SnapshotFormat
	if snapshotFormat != "" {
		format = regresql.SnapshotFormat(snapshotFormat)
	}

	detectedFormat := format
	if detectedFormat == "" {
		detectedFormat = regresql.DetectSnapshotFormat(inputPath)
	}
	if err := regresql.CheckPgTool(detectedFormat.RestoreTool(), snapshotCwd); err != nil {
		return err
	}

	opts := regresql.RestoreOptions{
		InputPath: inputPath,
		Format:    format,
		Clean:     snapshotClean,
	}

	fmt.Printf("Restoring database snapshot...\n")
	fmt.Printf("  Database: %s\n", maskConnectionString(cfg.PgUri))
	fmt.Printf("  Input:    %s\n", inputPath)
	if snapshotClean {
		fmt.Printf("  Mode:     clean (drop existing objects)\n")
	}
	fmt.Println()

	if err := regresql.RestoreSnapshot(cfg.PgUri, opts); err != nil {
		return err
	}

	fmt.Printf("Snapshot restored successfully.\n")
	return nil
}

func runSnapshotBuild() error {
	cfg, err := regresql.ReadConfig(snapshotCwd)
	if err != nil {
		return fmt.Errorf("failed to read config: %w (have you run 'regresql init'?)", err)
	}

	if cfg.PgUri == "" {
		return fmt.Errorf("pguri not configured in regress.yaml")
	}

	// Use config values as defaults when flags not provided
	schemaPath := snapshotBuildSchema
	if schemaPath == "" {
		schemaPath = regresql.GetSnapshotSchema(cfg.Snapshot)
	}
	if schemaPath != "" {
		if !filepath.IsAbs(schemaPath) {
			schemaPath = filepath.Join(snapshotCwd, schemaPath)
		}
		if _, err := os.Stat(schemaPath); err != nil {
			return fmt.Errorf("schema file not found: %s", schemaPath)
		}
	}

	migrationsDir := snapshotBuildMigrations
	if migrationsDir == "" {
		migrationsDir = regresql.GetSnapshotMigrations(cfg.Snapshot)
	}
	if migrationsDir != "" {
		if !filepath.IsAbs(migrationsDir) {
			migrationsDir = filepath.Join(snapshotCwd, migrationsDir)
		}
		if stat, err := os.Stat(migrationsDir); err != nil || !stat.IsDir() {
			return fmt.Errorf("migrations directory not found: %s", migrationsDir)
		}
	}

	migrationCommand := regresql.GetSnapshotMigrationCommand(cfg.Snapshot)

	// migrations dir and migration_command are mutually exclusive
	if migrationsDir != "" && migrationCommand != "" {
		return fmt.Errorf("cannot use both 'migrations' directory and 'migration_command' - choose one")
	}

	fixtures := snapshotBuildFixtures
	if len(fixtures) == 0 {
		fixtures = regresql.GetSnapshotFixtures(cfg.Snapshot)
	}

	// require at least schema, migrations, migration_command, or fixtures
	if len(fixtures) == 0 && schemaPath == "" && migrationsDir == "" && migrationCommand == "" {
		return fmt.Errorf("no schema, migrations, or fixtures specified. Use flags or configure in regress.yaml")
	}

	if len(fixtures) > 0 {
		if err := regresql.FixturesExist(snapshotCwd, fixtures); err != nil {
			return err
		}
	}

	outputPath := snapshotOutput
	if outputPath == "" {
		outputPath = regresql.GetSnapshotPath(cfg.Snapshot, snapshotCwd)
	} else if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(snapshotCwd, outputPath)
	}

	var format regresql.SnapshotFormat
	if snapshotFormat != "" {
		format = regresql.SnapshotFormat(snapshotFormat)
	} else {
		format = regresql.GetSnapshotFormat(cfg.Snapshot)
	}

	fmt.Printf("Building snapshot...\n")
	fmt.Printf("  Database: %s\n", maskConnectionString(cfg.PgUri))
	fmt.Printf("  Output:   %s\n", outputPath)
	fmt.Printf("  Format:   %s\n", format)
	if schemaPath != "" {
		fmt.Printf("  Schema:   %s\n", schemaPath)
	}
	if migrationsDir != "" {
		fmt.Printf("  Migrations: %s\n", migrationsDir)
	}
	if migrationCommand != "" {
		fmt.Printf("  Migration cmd: %s\n", migrationCommand)
	}
	if len(fixtures) > 0 {
		fmt.Printf("  Fixtures: %v\n", fixtures)
	}
	fmt.Println()

	result, err := regresql.BuildSnapshot(cfg.PgUri, snapshotCwd, regresql.SnapshotBuildOptions{
		OutputPath:       outputPath,
		Format:           format,
		SchemaPath:       schemaPath,
		MigrationsDir:    migrationsDir,
		MigrationCommand: migrationCommand,
		Fixtures:         fixtures,
		Verbose:          snapshotBuildVerbose,
	})
	if err != nil {
		return err
	}

	snapshotsDir := filepath.Dir(outputPath)
	if err := regresql.WriteSnapshotMetadata(snapshotsDir, result.Info); err != nil {
		fmt.Printf("Warning: failed to write snapshot metadata: %s\n", err)
	}

	fmt.Printf("Snapshot built successfully.\n")
	fmt.Printf("  Size:     %s\n", regresql.FormatBytes(result.Info.SizeBytes))
	fmt.Printf("  Hash:     %s\n", result.Info.Hash)
	fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))
	if result.Info.SchemaHash != "" {
		fmt.Printf("  Schema:   %s\n", result.Info.SchemaHash[:20]+"...")
	}
	if len(result.Info.MigrationsApplied) > 0 {
		fmt.Printf("  Migrations: %d applied\n", len(result.Info.MigrationsApplied))
	}
	if result.Info.MigrationCommandHash != "" {
		fmt.Printf("  Migration cmd: executed\n")
	}
	if len(result.FixturesUsed) > 0 {
		fmt.Printf("  Fixtures: %d applied\n", len(result.FixturesUsed))
	}

	return nil
}

func runSnapshotInfo() error {
	snapshotsDir := regresql.GetSnapshotsDir(snapshotCwd)

	metadata, err := regresql.ReadSnapshotMetadata(snapshotsDir)
	if err != nil {
		return fmt.Errorf("no snapshot metadata found. Run 'regresql snapshot build' or 'regresql snapshot capture' first")
	}

	if metadata.Current == nil {
		return fmt.Errorf("snapshot metadata is empty")
	}

	info := metadata.Current

	fmt.Printf("Snapshot: %s\n", info.Path)
	fmt.Printf("  Format:  %s\n", info.Format)
	fmt.Printf("  Size:    %s\n", regresql.FormatBytes(info.SizeBytes))
	fmt.Printf("  Created: %s\n", info.Created.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  Hash:    %s\n", info.Hash)

	if info.SchemaPath != "" {
		fmt.Println()
		fmt.Println("Schema:")
		fmt.Printf("  Path: %s\n", info.SchemaPath)
		fmt.Printf("  Hash: %s\n", info.SchemaHash)
	}

	if info.MigrationsDir != "" {
		fmt.Println()
		fmt.Println("Migrations:")
		fmt.Printf("  Dir:  %s\n", info.MigrationsDir)
		fmt.Printf("  Hash: %s\n", info.MigrationsHash)
		if len(info.MigrationsApplied) > 0 {
			fmt.Println("  Applied:")
			for _, m := range info.MigrationsApplied {
				fmt.Printf("    - %s\n", m)
			}
		}
	}

	if info.MigrationCommand != "" {
		fmt.Println()
		fmt.Println("Migration command:")
		fmt.Printf("  Command: %s\n", info.MigrationCommand)
		fmt.Printf("  Hash:    %s\n", info.MigrationCommandHash)
	}

	if len(info.FixturesUsed) > 0 {
		fmt.Println()
		fmt.Println("Fixtures used:")
		for _, f := range info.FixturesUsed {
			fmt.Printf("  - %s\n", f)
		}
	}

	return nil
}
