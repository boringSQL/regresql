package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	removeCwd    string
	removeClean  bool
	removeDryRun bool

	removeCmd = &cobra.Command{
		Use:   "remove <path...>",
		Short: "Remove SQL files from the test suite",
		Long: `Remove SQL files from the test suite by deleting plan files.

Paths can be:
  - Single file:  orders/get_order.sql
  - Directory:    orders/ (all SQL files within)
  - Glob pattern: orders/*.sql

Options:
  --clean    Also delete expected and baseline files
  --dry-run  Show what would be deleted without deleting

Examples:
  regresql remove orders/get_order.sql
  regresql remove orders/ --clean
  regresql remove orders/*.sql --dry-run`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(removeCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}

			opts := regresql.RemoveOptions{
				Root:   removeCwd,
				Paths:  args,
				Clean:  removeClean,
				DryRun: removeDryRun,
			}

			if err := regresql.RemoveQueries(opts); err != nil {
				fmt.Printf("Error: %s\n", err)
				os.Exit(1)
			}
		},
	}
)

func init() {
	RootCmd.AddCommand(removeCmd)

	removeCmd.Flags().StringVarP(&removeCwd, "cwd", "C", ".", "Change to directory")
	removeCmd.Flags().BoolVar(&removeClean, "clean", false, "Also delete expected and baseline files")
	removeCmd.Flags().BoolVar(&removeDryRun, "dry-run", false, "Show what would be deleted")
}
