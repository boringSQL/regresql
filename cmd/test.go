package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	testCwd       string
	testRunFilter string
)

// testCmd represents the test command
var testCmd = &cobra.Command{
	Use:   "test [flags]",
	Short: "Run regression tests for your SQL queries",
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkDirectory(testCwd); err != nil {
			fmt.Print(err.Error())
			os.Exit(1)
		}
		regresql.Test(testCwd, testRunFilter)
	},
}

func init() {
	RootCmd.AddCommand(testCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// testCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// testCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	testCmd.Flags().StringVarP(&testCwd, "cwd", "C", ".", "Change to Directory")
	testCmd.Flags().StringVar(&testRunFilter, "run", "", "Run only queries matching regexp (matches file names and query names)")
}
