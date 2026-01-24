package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	planCmd = &cobra.Command{
		Use:    "plan",
		Short:  "Deprecated: use 'regresql add' instead",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(os.Stderr, `Error: 'regresql plan' is deprecated.

Use 'regresql add' instead:
  regresql add <path>       Add specific files
  regresql add .            Add all SQL files in current directory
  regresql discover         List SQL files and their status

See 'regresql add --help' for more information.`)
			os.Exit(1)
		},
	}
)

func init() {
	RootCmd.AddCommand(planCmd)
}
