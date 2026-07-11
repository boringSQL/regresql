package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/boringsql/regresql/v2/regresql"
	"github.com/spf13/cobra"
)

var (
	compareCwd           string
	compareBaseURI       string
	compareTargetURI     string
	compareRunFilter     string
	compareFormat        string
	compareOutput        string
	compareWarmups       int
	compareAdmit         bool
	compareAdmitReps     int
	compareSamples       int
	compareTimeout       time.Duration
	compareInjectStats   bool
	compareStability     bool
	compareStabilityReps int

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
				Root:          compareCwd,
				BaseURI:       compareBaseURI,
				TargetURI:     compareTargetURI,
				RunFilter:     compareRunFilter,
				Format:        compareFormat,
				OutputPath:    compareOutput,
				Warmups:       compareWarmups,
				Admit:         compareAdmit,
				AdmitReps:     compareAdmitReps,
				Samples:       compareSamples,
				Timeout:       compareTimeout,
				InjectStats:   compareInjectStats,
				Stability:     compareStability,
				StabilityReps: compareStabilityReps,
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
	compareCmd.Flags().IntVar(&compareWarmups, "warmups", 2, "Discarded EXPLAIN ANALYZE runs before the measured one (warm buffer cache for fair comparison)")
	compareCmd.Flags().BoolVar(&compareAdmit, "admit", false, "Preflight: exclude queries whose result isn't plan-invariant (determinism filter)")
	compareCmd.Flags().IntVar(&compareAdmitReps, "admit-reps", regresql.DefaultAdmitReps, "Repetitions per perturbation in the --admit preflight")
	compareCmd.Flags().IntVar(&compareSamples, "samples", 0, "Interleaved timing runs per engine (0 = no timing; advisory, self-calibrated)")
	compareCmd.Flags().DurationVar(&compareTimeout, "timeout", 0, "Per-query statement_timeout so a pathological plan can't stall the run (e.g. 60s; 0 = none)")
	compareCmd.Flags().BoolVar(&compareInjectStats, "inject-stats", false, "Copy base statistics into target first so diffs are planner code, not ANALYZE noise (needs pg_dump/psql; mutates target stats)")
	compareCmd.Flags().BoolVar(&compareStability, "stability", false, "Exclude cost-tie queries whose plan swings on re-ANALYZE (mutates base stats)")
	compareCmd.Flags().IntVar(&compareStabilityReps, "stability-reps", regresql.DefaultStabilityReps, "Re-ANALYZE repetitions for the --stability preflight")
}
