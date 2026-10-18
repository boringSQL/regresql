package cli

import (
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version = "dev" // Will be set via ldflags during build

	// RootCmd represents the base command when called without any subcommands
	RootCmd = &cobra.Command{
		Use:           "regresql",
		Short:         "Run regression tests for your SQL queries",
		SilenceErrors: true, // main prints the error once
	}
)

func init() {
	info, _ := debug.ReadBuildInfo()
	version = buildVersion(version, info)
	RootCmd.Version = version
}

// buildVersion keeps a version set by ldflags and otherwise falls back to the
// module version Go stamps into the binary (`go install pkg@vX.Y.Z`, or a
// clean checkout build).
func buildVersion(v string, info *debug.BuildInfo) string {
	if v != "dev" || info == nil {
		return v
	}
	mv := info.Main.Version
	if mv == "" || mv == "(devel)" || strings.HasSuffix(mv, "+dirty") {
		return v
	}
	return mv
}

// Run executes the root command. Child commands register themselves via
// init() in their respective files.
func Run() error {
	return RootCmd.Execute()
}
