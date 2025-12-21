package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	testCwd        string
	testRunFilter  string
	testFormat     string
	testOutputPath string
	testCommit     bool
	testNoRestore  bool

	testCmd = &cobra.Command{
		Use:   "test [flags]",
		Short: "Run regression tests for your SQL queries",
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(testCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			regresql.Test(testCwd, testRunFilter, testFormat, testOutputPath, testCommit, testNoRestore)
		},
	}
)

func init() {
	RootCmd.AddCommand(testCmd)

	testCmd.Flags().StringVarP(&testCwd, "cwd", "C", ".", "Change to Directory")
	testCmd.Flags().StringVar(&testRunFilter, "run", "", "Run only queries matching regexp (matches file names and query names)")
	testCmd.Flags().StringVar(&testFormat, "format", "console", "Output format: console, pgtap, junit, json, github-actions")
	testCmd.Flags().StringVarP(&testOutputPath, "output", "o", "", "Output file path (default: stdout)")
	testCmd.Flags().BoolVar(&testCommit, "commit", false, "Commit transactions instead of rollback (use with caution)")
	testCmd.Flags().BoolVar(&testNoRestore, "no-restore", false, "Skip snapshot restore before test")
}
