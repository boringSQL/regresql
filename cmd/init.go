package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

// Command Flags
var (
	pguri string
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init [flags] postgres://user@host/dbname",
	Short: "Initialize regresql for use in your project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkDirectory(cwd); err != nil {
			fmt.Printf(err.Error())
			os.Exit(1)
		}
		pguri := args[0]
		regresql.Init(cwd, pguri)
	},
}

func init() {
	RootCmd.AddCommand(initCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	initCmd.Flags().StringVarP(&cwd, "cwd", "C", ".", "Change to Directory")
}
