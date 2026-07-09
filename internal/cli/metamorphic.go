package cli

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/v2/regresql"
	"github.com/spf13/cobra"
)

var (
	metaCwd       string
	metaURI       string
	metaRunFilter string
	metaFormat    string
	metaOutput    string

	metamorphicCmd = &cobra.Command{
		Use:   "metamorphic [flags]",
		Short: "Check that turning optimizations off doesn't change your results",
		Long: `Flips result-preserving optimizer settings (eager aggregation, memoize,
incremental sort, partitionwise join/aggregate) off one at a time and checks the
query still returns the same rows. A change is a wrong-results bug in the
optimizer. Run 'admit' first so plan-dependent queries don't show up as bugs.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(metaCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			code := regresql.Metamorphic(regresql.MetamorphicOptions{
				Root:       metaCwd,
				URI:        metaURI,
				RunFilter:  metaRunFilter,
				Format:     metaFormat,
				OutputPath: metaOutput,
			})
			os.Exit(code)
		},
	}
)

func init() {
	RootCmd.AddCommand(metamorphicCmd)

	metamorphicCmd.Flags().StringVarP(&metaCwd, "cwd", "C", ".", "Change to Directory")
	metamorphicCmd.Flags().StringVar(&metaURI, "uri", "", "Server connection string (default: config pguri)")
	metamorphicCmd.Flags().StringVar(&metaRunFilter, "run", "", "Run only queries matching regexp")
	metamorphicCmd.Flags().StringVar(&metaFormat, "format", "console", "Output format: console, json")
	metamorphicCmd.Flags().StringVarP(&metaOutput, "output", "o", "", "Output file path (default: stdout)")
}
