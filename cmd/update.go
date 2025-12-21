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

	// updateCmd represents the update command
	updateCmd = &cobra.Command{
		Use:   "update [flags]",
		Short: "Creates or updates the expected output files",
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(updateCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			regresql.Update(updateCwd, updateRunFilter, updateCommit, updateNoRestore, updateForceRestore)
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
}
