package cli

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/v2/regresql"
	"github.com/spf13/cobra"
)

var (
	admitCwd       string
	admitURI       string
	admitRunFilter string
	admitFormat    string
	admitOutput    string
	admitReps      int
	admitStrict    bool

	admitCmd = &cobra.Command{
		Use:   "admit [flags]",
		Short: "Drop queries whose result changes when the plan changes",
		Long: `Runs each query under different plans (enable_* flips, parallel, tiny work_mem),
a few times each, and keeps only the ones that return the same rows every time.
A query whose result depends on the plan would give a false signal in compare.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(admitCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			code := regresql.Admit(regresql.AdmitOptions{
				Root:       admitCwd,
				URI:        admitURI,
				RunFilter:  admitRunFilter,
				Format:     admitFormat,
				OutputPath: admitOutput,
				Reps:       admitReps,
				Strict:     admitStrict,
			})
			os.Exit(code)
		},
	}
)

func init() {
	RootCmd.AddCommand(admitCmd)

	admitCmd.Flags().StringVarP(&admitCwd, "cwd", "C", ".", "Change to Directory")
	admitCmd.Flags().StringVar(&admitURI, "uri", "", "Server connection string (default: config pguri)")
	admitCmd.Flags().StringVar(&admitRunFilter, "run", "", "Run only queries matching regexp")
	admitCmd.Flags().StringVar(&admitFormat, "format", "console", "Output format: console, json")
	admitCmd.Flags().StringVarP(&admitOutput, "output", "o", "", "Output file path (default: stdout)")
	admitCmd.Flags().IntVar(&admitReps, "reps", regresql.DefaultAdmitReps, "Repetitions per perturbation (catches run-to-run nondeterminism)")
	admitCmd.Flags().BoolVar(&admitStrict, "strict", false, "Exit 1 if any query is rejected")
}
