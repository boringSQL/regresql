package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	discoverCwd     string
	discoverQueries bool

	discoverCmd = &cobra.Command{
		Use:   "discover",
		Short: "List SQL files and their test status",
		Long: `Discover SQL files in the project and show their test status.

Each file is shown with a status indicator:
  [+] All queries have plan files
  [ ] No queries have plan files
  [~] Some queries have plan files (partial - new queries added to file?)

Use --queries to see individual query status within each file.
Use 'regresql add <path>' to add files to the test suite.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(discoverCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}

			opts := regresql.DiscoverOptions{
				Root:       discoverCwd,
				ShowDetail: discoverQueries,
			}

			results, err := regresql.Discover(opts)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				os.Exit(1)
			}

			if len(results) == 0 {
				fmt.Println("No SQL files found in project.")
				return
			}

			regresql.PrintDiscoveryResults(results, discoverQueries)
		},
	}
)

func init() {
	RootCmd.AddCommand(discoverCmd)

	discoverCmd.Flags().StringVarP(&discoverCwd, "cwd", "C", ".", "Change to directory")
	discoverCmd.Flags().BoolVar(&discoverQueries, "queries", false, "Show query-level detail")
}
