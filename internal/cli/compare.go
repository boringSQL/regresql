package cli

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/v2/regresql"
	"github.com/spf13/cobra"
)

var (
	compareCwd       string
	compareBaseURI   string
	compareTargetURI string
	compareRunFilter string
	compareFormat    string
	compareOutput    string

	compareCmd = &cobra.Command{
		Use:   "compare --base <uri> --target <uri>",
		Short: "Diff the corpus across two PostgreSQL builds (planner A/B)",
		Long: `Run the query corpus against two live servers and diff results, plan shape,
buffers, spill, tuple flow and per-node q-error. Estimated cost is suppressed
when the server versions differ. Emits a scoreboard for a patch cover letter.`,
		Run: func(cmd *cobra.Command, args []string) {
			if compareBaseURI == "" || compareTargetURI == "" {
				fmt.Fprintln(os.Stderr, "both --base and --target are required")
				os.Exit(2)
			}
			if err := checkDirectory(compareCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			code := regresql.Compare(regresql.CompareOptions{
				Root:       compareCwd,
				BaseURI:    compareBaseURI,
				TargetURI:  compareTargetURI,
				RunFilter:  compareRunFilter,
				Format:     compareFormat,
				OutputPath: compareOutput,
			})
			os.Exit(code)
		},
	}
)

func init() {
	RootCmd.AddCommand(compareCmd)

	compareCmd.Flags().StringVarP(&compareCwd, "cwd", "C", ".", "Change to Directory")
	compareCmd.Flags().StringVar(&compareBaseURI, "base", "", "Base (reference) server connection string")
	compareCmd.Flags().StringVar(&compareTargetURI, "target", "", "Target (candidate) server connection string")
	compareCmd.Flags().StringVar(&compareRunFilter, "run", "", "Run only queries matching regexp")
	compareCmd.Flags().StringVar(&compareFormat, "format", "console", "Output format: console, markdown, json")
	compareCmd.Flags().StringVarP(&compareOutput, "output", "o", "", "Output file path (default: stdout)")
}
