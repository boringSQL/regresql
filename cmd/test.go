package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	testCwd           string
	testRunFilter     string
	testFormat        string
	testOutputPath    string
	testCommit        bool
	testNoRestore     bool
	testFailOnSkipped bool
	testColor         bool
	testNoColor       bool
	testFullDiff      bool
	testNoDiff        bool
	testSnapshot  string
	testStatsFile string
	testVerbose   bool

	testCmd = &cobra.Command{
		Use:   "test [flags]",
		Short: "Run regression tests for your SQL queries",
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(testCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			opts := regresql.TestOptions{
				Root:          testCwd,
				RunFilter:     testRunFilter,
				FormatName:    testFormat,
				OutputPath:    testOutputPath,
				Commit:        testCommit,
				NoRestore:     testNoRestore,
				FailOnSkipped: testFailOnSkipped,
				Color:         testColor,
				NoColor:       testNoColor,
				FullDiff:      testFullDiff,
				NoDiff:        testNoDiff,
				Snapshot:      testSnapshot,
				StatsFile:     testStatsFile,
				Verbose:       testVerbose,
			}
			regresql.Test(opts)
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
	testCmd.Flags().BoolVar(&testFailOnSkipped, "fail-on-skipped", false, "Exit with code 2 if skipped tests exist")
	testCmd.Flags().BoolVar(&testColor, "color", false, "Force colored output")
	testCmd.Flags().BoolVar(&testNoColor, "no-color", false, "Disable colored output")
	testCmd.Flags().BoolVar(&testFullDiff, "diff", false, "Show full diff output (no truncation)")
	testCmd.Flags().BoolVar(&testNoDiff, "no-diff", false, "Suppress diff output entirely")
	testCmd.Flags().StringVar(&testSnapshot, "snapshot", "", "Run tests against specific snapshot (tag or hash prefix)")
	testCmd.Flags().StringVar(&testStatsFile, "stats", "", "Statistics file to apply instead of ANALYZE (requires PG18+)")
	testCmd.Flags().BoolVarP(&testVerbose, "verbose", "v", false, "Show each test with name, type, and duration")
}
