package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	planCwd       string
	planRunFilter string
)

// planCmd represents the plan command
var planCmd = &cobra.Command{
	Use:   "plan [flags]",
	Short: "Creates missing plans for new queries",
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkDirectory(planCwd); err != nil {
			fmt.Print(err.Error())
			os.Exit(1)
		}
		regresql.PlanQueries(planCwd, planRunFilter)
	},
}

func init() {
	RootCmd.AddCommand(planCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// planCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// planCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	planCmd.Flags().StringVarP(&planCwd, "cwd", "C", ".", "Change to Directory")
	planCmd.Flags().StringVar(&planRunFilter, "run", "", "Run only queries matching regexp (matches file names and query names)")
}
