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
		Use:   "baseline [flags]",
		Short: "Creates baseline EXPLAIN analysis for queries",
		Long: `Creates baseline EXPLAIN analysis for all queries in the suite.
This command executes EXPLAIN for each query and stores the query plan
metrics (costs, timing, rows) in JSON files under the baselines directory.

Use --analyze to create baselines with EXPLAIN (ANALYZE, BUFFERS) which
captures actual buffer I/O counts for deterministic regression detection.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(baselineCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			regresql.BaselineQueries(baselineCwd, baselineRunFilter, baselineAnalyze)
		},
	}
)

func init() {
	RootCmd.AddCommand(baselineCmd)

	baselineCmd.Flags().StringVarP(&baselineCwd, "cwd", "C", ".", "Change to Directory")
	baselineCmd.Flags().StringVar(&baselineRunFilter, "run", "", "Run only queries matching regexp (matches file names and query names)")
	baselineCmd.Flags().BoolVar(&baselineAnalyze, "analyze", false, "Use EXPLAIN (ANALYZE, BUFFERS) for baselines")
}
