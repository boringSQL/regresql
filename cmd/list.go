package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	// Command Flags
	cwd string

	// listCmd represents the list command
	listCmd = &cobra.Command{
		Use:   "list",
		Short: "list candidates SQL files",
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(cwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			regresql.List(cwd)
		},
	}
)

func init() {
	RootCmd.AddCommand(listCmd)
	listCmd.Flags().StringVarP(&cwd, "cwd", "C", ".", "Change to Directory")
}
