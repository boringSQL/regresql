package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	updateCwd          string
	updateRunFilter    string
	updateCommit       bool
	updateNoRestore    bool
	updateForceRestore bool
	updatePending      bool
	updateInteractive  bool
	updateDryRun       bool

	// updateCmd represents the update command
	updateCmd = &cobra.Command{
		Use:   "update [path...]",
		Short: "Creates or updates the expected output files",
		Long: `Creates or updates the expected output files for queries.

Without arguments, updates all queries. With path arguments, only updates
queries matching those paths.

Examples:
  regresql update                         # Update all queries
  regresql update orders/                 # Update queries in orders/
  regresql update orders/get_order.sql    # Update specific query
  regresql update --pending               # Only create missing baselines
  regresql update --dry-run               # Preview what would be updated
  regresql update --interactive           # Review each change`,
		Args: cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(updateCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			regresql.Update(regresql.UpdateOptions{
				Root:         updateCwd,
				RunFilter:    updateRunFilter,
				Paths:        args,
				Commit:       updateCommit,
				NoRestore:    updateNoRestore,
				ForceRestore: updateForceRestore,
				Pending:      updatePending,
				Interactive:  updateInteractive,
				DryRun:       updateDryRun,
			})
		},
	}
)

func init() {
	RootCmd.AddCommand(updateCmd)

	updateCmd.Flags().StringVarP(&updateCwd, "cwd", "C", ".", "Change to Directory")
	updateCmd.Flags().StringVar(&updateRunFilter, "run", "", "Run only queries matching regexp (matches file names and query names)")
	updateCmd.Flags().BoolVar(&updateCommit, "commit", false, "Commit transactions instead of rollback (use with caution)")
	updateCmd.Flags().BoolVar(&updateNoRestore, "no-restore", false, "Skip snapshot restore before update")
	updateCmd.Flags().BoolVar(&updateForceRestore, "force-restore", false, "Force snapshot restore even if unchanged")
	updateCmd.Flags().BoolVar(&updatePending, "pending", false, "Only create baselines for queries without expected files")
	updateCmd.Flags().BoolVar(&updateInteractive, "interactive", false, "Review and confirm each update")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show what would be updated without writing files")
}
