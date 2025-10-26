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

	// baselineCmd represents the baseline command
	baselineCmd = &cobra.Command{
		Use:   "baseline [flags]",
		Short: "Creates baseline EXPLAIN analysis for queries",
		Long: `Creates baseline EXPLAIN analysis for all queries in the suite.
This command executes EXPLAIN for each query and stores the query plan
metrics (costs, timing, rows) in JSON files under the baselines directory.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(baselineCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			regresql.BaselineQueries(baselineCwd, baselineRunFilter)
		},
	}
)

func init() {
	RootCmd.AddCommand(baselineCmd)

	baselineCmd.Flags().StringVarP(&baselineCwd, "cwd", "C", ".", "Change to Directory")
	baselineCmd.Flags().StringVar(&baselineRunFilter, "run", "", "Run only queries matching regexp (matches file names and query names)")
}
