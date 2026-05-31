package cli

import (
	"github.com/spf13/cobra"
)

var (
	version = "dev" // Will be set via ldflags during build

	// RootCmd represents the base command when called without any subcommands
	RootCmd = &cobra.Command{
		Use:     "regresql",
		Short:   "Run regression tests for your SQL queries",
		Version: version,
	}
)

// Run executes the root command. Child commands register themselves via
// init() in their respective files.
func Run() error {
	return RootCmd.Execute()
}
