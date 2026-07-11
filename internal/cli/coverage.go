package cli

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/v2/regresql"
	"github.com/spf13/cobra"
)

var (
	coverageCwd      string
	coverageTaxonomy string
	coverageFormat   string
	coverageOutput   string

	coverageCmd = &cobra.Command{
		Use:   "coverage --taxonomy <file>",
		Short: "Report which planner-feature cells the corpus covers (and which it misses)",
		Long: `Cross-reference each query's -- cell: tag against a taxonomy of planner-feature
cells and report covered vs empty cells, tags not in the taxonomy, and untagged
queries. Honest coverage accounting — the empty cells are the point.`,
		Run: func(cmd *cobra.Command, args []string) {
			if coverageTaxonomy == "" {
				fmt.Fprintln(os.Stderr, "--taxonomy is required")
				os.Exit(2)
			}
			if err := checkDirectory(coverageCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			os.Exit(regresql.Coverage(regresql.CoverageOptions{
				Root:         coverageCwd,
				TaxonomyPath: coverageTaxonomy,
				Format:       coverageFormat,
				OutputPath:   coverageOutput,
			}))
		},
	}
)

func init() {
	RootCmd.AddCommand(coverageCmd)

	coverageCmd.Flags().StringVarP(&coverageCwd, "cwd", "C", ".", "Change to Directory")
	coverageCmd.Flags().StringVar(&coverageTaxonomy, "taxonomy", "", "Path to the taxonomy JSON (axes -> cells)")
	coverageCmd.Flags().StringVar(&coverageFormat, "format", "console", "Output format: console, json")
	coverageCmd.Flags().StringVarP(&coverageOutput, "output", "o", "", "Output file path (default: stdout)")
}
