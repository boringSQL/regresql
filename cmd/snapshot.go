package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	snapshotCwd        string
	snapshotOutput     string
	snapshotFormat     string
	snapshotSchemaOnly bool
	snapshotSection    string

	snapshotCmd = &cobra.Command{
		Use:   "snapshot",
		Short: "Manage database snapshots",
		Long: `Manage database snapshots for reproducible testing.

Snapshots capture the database state using pg_dump and can be restored
before running tests to ensure a consistent starting point.`,
	}

	snapshotCaptureCmd = &cobra.Command{
		Use:   "capture [flags]",
		Short: "Capture current database state as a snapshot",
		Long: `Capture the current database state using pg_dump.

The snapshot is stored in the snapshots directory and can be restored
before running tests. By default, the snapshot is saved in pg_dump custom
format which is efficient and portable.

Examples:
  # Capture with default settings (custom format)
  regresql snapshot capture

  # Capture to a specific file
  regresql snapshot capture --output snapshots/mydata.dump

  # Capture schema only (no data)
  regresql snapshot capture --schema-only

  # Capture in plain SQL format (git-friendly)
  regresql snapshot capture --format plain --output snapshots/schema.sql

  # Capture only a specific section
  regresql snapshot capture --section pre-data --output snapshots/schema-only.sql`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(snapshotCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}

			if err := runSnapshotCapture(); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}
)

func init() {
	RootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotCaptureCmd)

	// Shared flags for snapshot commands
	snapshotCmd.PersistentFlags().StringVarP(&snapshotCwd, "cwd", "C", ".", "Change to directory")

	// Capture-specific flags
	snapshotCaptureCmd.Flags().StringVarP(&snapshotOutput, "output", "o", "", "Output file path (default: from config or snapshots/default.dump)")
	snapshotCaptureCmd.Flags().StringVarP(&snapshotFormat, "format", "f", "", "Dump format: custom, plain, or directory (default: custom)")
	snapshotCaptureCmd.Flags().BoolVar(&snapshotSchemaOnly, "schema-only", false, "Dump only schema, no data")
	snapshotCaptureCmd.Flags().StringVar(&snapshotSection, "section", "", "Dump specific section: pre-data, data, or post-data")
}

func runSnapshotCapture() error {
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
