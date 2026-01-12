package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	migrateCwd      string
	migrateScript   string
	migrateCommand  string
	migrateKeepTemp bool
	migrateVerbose  bool
	migrateColor    bool
	migrateNoColor  bool
	migrateFullDiff bool
	migrateNoDiff   bool

	migrateCmd = &cobra.Command{
		Use:   "migrate [flags]",
		Short: "Test migration impact on query outputs",
		Long: `Execute all queries before and after a migration to detect output changes.

Workflow:
  1. Restore database from snapshot
  2. Execute all queries -> capture "before" state
  3. Apply migration (script or command)
  4. Execute all queries -> capture "after" state
  5. Compare and report differences

Examples:
  regresql migrate --script migrations/002_add_status.sql
  regresql migrate --command "goose -dir migrations postgres \$PGURI up-to 002"
  regresql migrate --script migrations/002.sql --verbose`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(migrateCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}

			// Validate: exactly one of --script or --command required
			if migrateScript == "" && migrateCommand == "" {
				fmt.Println("Error: either --script or --command is required")
				os.Exit(1)
			}
			if migrateScript != "" && migrateCommand != "" {
				fmt.Println("Error: --script and --command are mutually exclusive")
				os.Exit(1)
			}

			// Validate script file exists if specified
			if migrateScript != "" {
				if _, err := os.Stat(migrateScript); os.IsNotExist(err) {
					fmt.Printf("Error: migration script not found: %s\n", migrateScript)
					os.Exit(1)
				}
			}

			opts := regresql.MigrateOptions{
				Root:     migrateCwd,
				Script:   migrateScript,
				Command:  migrateCommand,
				KeepTemp: migrateKeepTemp,
				Verbose:  migrateVerbose,
				Color:    migrateColor,
				NoColor:  migrateNoColor,
				FullDiff: migrateFullDiff,
				NoDiff:   migrateNoDiff,
			}
			exitCode := regresql.Migrate(opts)
			os.Exit(exitCode)
		},
	}
)

func init() {
	RootCmd.AddCommand(migrateCmd)

	migrateCmd.Flags().StringVarP(&migrateCwd, "cwd", "C", ".", "Change to Directory")
	migrateCmd.Flags().StringVar(&migrateScript, "script", "", "Path to migration SQL script")
	migrateCmd.Flags().StringVar(&migrateCommand, "command", "", "External migration command (receives $PGURI env var)")
	migrateCmd.Flags().BoolVar(&migrateKeepTemp, "keep-temp", false, "Preserve temporary before/after directories")
	migrateCmd.Flags().BoolVarP(&migrateVerbose, "verbose", "v", false, "Verbose output")
	migrateCmd.Flags().BoolVar(&migrateColor, "color", false, "Force colored output")
	migrateCmd.Flags().BoolVar(&migrateNoColor, "no-color", false, "Disable colored output")
	migrateCmd.Flags().BoolVar(&migrateFullDiff, "diff", false, "Show full diff output (no truncation)")
	migrateCmd.Flags().BoolVar(&migrateNoDiff, "no-diff", false, "Suppress diff output entirely")
}
