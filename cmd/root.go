package cmd

import (
	"fmt"
	"os"

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

// Execute adds all child commands to the root command and sets flags
// appropriately. This is called by main.main(). It only needs to happen
// once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
