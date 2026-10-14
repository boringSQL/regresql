package cli

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/v2/regresql"
	"github.com/spf13/cobra"
)

var (
	doctorCwd    string
	doctorStats  string
	doctorFormat string

	doctorCmd = &cobra.Command{
		Use:   "doctor [flags]",
		Short: "Report assertion mode and environment readiness",
		Long: `Report which assertion mode regresql will run in and whether the
environment can support it: connection, pg_regresql availability, stats
injection and the PostgreSQL major of the stats file against the server.
Read-only: nothing is restored or written.

Exit codes: 0 healthy, 2 connection error, 3 config error,
15 environment not fit to run.

Examples:
  regresql doctor
  regresql doctor --stats stats.sql --format json`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(doctorCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			env, err := regresql.Doctor(regresql.DoctorOptions{
				Root:   doctorCwd,
				Stats:  doctorStats,
				Format: doctorFormat,
				Out:    os.Stdout,
			})
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			os.Exit(doctorExit(env, err))
		},
	}
)

func init() {
	RootCmd.AddCommand(doctorCmd)

	doctorCmd.Flags().StringVarP(&doctorCwd, "cwd", "C", ".", "Change to directory")
	doctorCmd.Flags().StringVar(&doctorStats, "stats", "", "SQL statistics file to check (default: stats.default from config)")
	doctorCmd.Flags().StringVar(&doctorFormat, "format", "text", "Output format: text, json")
}

func doctorExit(env regresql.EnvStatus, err error) int {
	switch {
	case err != nil:
		return 3
	case !env.Connected:
		return 2
	case env.Failed():
		return 15
	}
	return 0
}
