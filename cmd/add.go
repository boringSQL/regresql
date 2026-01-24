package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	addCwd   string
	addForce bool

	addCmd = &cobra.Command{
		Use:   "add <path...>",
		Short: "Add SQL files to the test suite",
		Long: `Add SQL files to the test suite by creating plan files.

Paths can be:
  - Single file:  orders/get_order.sql
  - Directory:    orders/ (all SQL files within)
  - Glob pattern: orders/*.sql

Examples:
  regresql add orders/get_order.sql
  regresql add orders/
  regresql add orders/*.sql
  regresql add .  # Add all SQL files

After adding, edit the plan files to add parameter values, then run
'regresql update' to generate expected outputs.`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(addCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}

			opts := regresql.AddOptions{
				Root:  addCwd,
				Paths: args,
				Force: addForce,
			}

			if err := regresql.AddQueries(opts); err != nil {
				fmt.Printf("Error: %s\n", err)
				os.Exit(1)
			}
		},
	}
)

func init() {
	RootCmd.AddCommand(addCmd)

	addCmd.Flags().StringVarP(&addCwd, "cwd", "C", ".", "Change to directory")
	addCmd.Flags().BoolVar(&addForce, "force", false, "Overwrite existing plan files")
}
