package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	updateCwd       string
	updateRunFilter string
)

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update [flags]",
	Short: "Creates or updates the expected output files",
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkDirectory(updateCwd); err != nil {
			fmt.Print(err.Error())
			os.Exit(1)
		}
		regresql.Update(updateCwd, updateRunFilter)
	},
}

func init() {
	RootCmd.AddCommand(updateCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// updateCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// updateCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	updateCmd.Flags().StringVarP(&updateCwd, "cwd", "C", ".", "Change to Directory")
	updateCmd.Flags().StringVar(&updateRunFilter, "run", "", "Run only queries matching regexp (matches file names and query names)")
}
