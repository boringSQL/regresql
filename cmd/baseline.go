package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	baselineCwd       string
	baselineRunFilter string
	baselineAnalyze   bool

	// baselineCmd represents the baseline command
	baselineCmd = &cobra.Command{
		Use:   "baseline [path...] [flags]",
		Short: "Creates baseline EXPLAIN analysis for queries",
		Long: `Creates baseline EXPLAIN analysis for queries in the suite.

Without arguments, creates baselines for all queries. With path arguments,
only creates baselines for queries matching those paths.

Examples:
  regresql baseline                         # All queries
  regresql baseline orders/                 # Queries in orders/
  regresql baseline orders/get_order.sql    # Specific query

Use --analyze to create baselines with EXPLAIN (ANALYZE, BUFFERS) which
captures actual buffer I/O counts for deterministic regression detection.`,
		Args: cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(baselineCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			regresql.BaselineQueries(regresql.BaselineOptions{
				Root:      baselineCwd,
				RunFilter: baselineRunFilter,
				Analyze:   baselineAnalyze,
				Paths:     args,
			})
		},
	}
)

func init() {
	RootCmd.AddCommand(baselineCmd)

	baselineCmd.Flags().StringVarP(&baselineCwd, "cwd", "C", ".", "Change to Directory")
	baselineCmd.Flags().StringVar(&baselineRunFilter, "run", "", "Run only queries matching regexp (matches file names and query names)")
	baselineCmd.Flags().BoolVar(&baselineAnalyze, "analyze", false, "Use EXPLAIN (ANALYZE, BUFFERS) for baselines")
}
