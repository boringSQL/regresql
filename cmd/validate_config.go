package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	validateConfigCwd string

	validateConfigCmd = &cobra.Command{
		Use:   "validate-config",
		Short: "Validate configuration for RegreSQL 2.0 compatibility",
		Long: `Checks configuration files for deprecated patterns and validates
readiness for RegreSQL 2.0.

Validates:
  - Config file exists and is parseable
  - No deprecated 'fixtures:' or 'cleanup:' fields in plan files
  - All fixture files are valid
  - Snapshot paths exist (if configured)`,
		Run: runValidateConfig,
	}
)

func init() {
	RootCmd.AddCommand(validateConfigCmd)
	validateConfigCmd.Flags().StringVarP(&validateConfigCwd, "cwd", "C", ".", "Change to Directory")
}

func runValidateConfig(cmd *cobra.Command, args []string) {
	if err := checkDirectory(validateConfigCwd); err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	fmt.Println("Checking configuration...")
	fmt.Println()

	result := regresql.ValidateForUpgrade(validateConfigCwd)

	if result.ConfigValid {
		fmt.Printf("✓ Config file found: %s\n", result.ConfigFile)
	} else {
		fmt.Printf("✗ Config file error: %s\n", result.ConfigError)
	}

	printPlanIssues(result.PlanIssues)
	printFixtureIssues(result.FixtureIssues, result.FixtureCount)
	printSnapshotIssues(result.SnapshotIssues)

	fmt.Println()
	if result.Passed {
		fmt.Println("✓ Ready for RegreSQL 2.0")
		os.Exit(0)
	} else {
		fmt.Println("✗ Not ready for RegreSQL 2.0")
		fmt.Println("  Fix the issues above before upgrading.")
		os.Exit(1)
	}
}

func printPlanIssues(issues []regresql.ValidationIssue) {
	fixtureIssues := filterByField(issues, "fixtures")
	cleanupIssues := filterByField(issues, "cleanup")

	if len(fixtureIssues) == 0 {
		fmt.Println("✓ No deprecated per-test fixtures in plan files")
	} else {
		fmt.Println("✗ Deprecated 'fixtures:' found in plan files:")
		for _, issue := range fixtureIssues {
			fmt.Printf("  - %s\n", issue.File)
		}
	}

	if len(cleanupIssues) == 0 {
		fmt.Println("✓ No deprecated cleanup strategies in plan files")
	} else {
		fmt.Println("✗ Deprecated 'cleanup:' found in plan files:")
		for _, issue := range cleanupIssues {
			fmt.Printf("  - %s\n", issue.File)
		}
	}
}

func printFixtureIssues(issues []regresql.ValidationIssue, count int) {
	if len(issues) == 0 {
		if count > 0 {
			fmt.Printf("✓ Fixture files valid (%d files)\n", count)
		} else {
			fmt.Println("✓ No fixture files to validate")
		}
	} else {
		fmt.Println("✗ Fixture file issues:")
		for _, issue := range issues {
			fmt.Printf("  - %s: %s\n", issue.File, issue.Message)
		}
	}
}

func printSnapshotIssues(issues []regresql.ValidationIssue) {
	if len(issues) == 0 {
		return
	}

	fmt.Println("✗ Snapshot configuration issues:")
	for _, issue := range issues {
		fmt.Printf("  - %s: %s\n", issue.Field, issue.Message)
	}
}

func filterByField(issues []regresql.ValidationIssue, field string) []regresql.ValidationIssue {
	var filtered []regresql.ValidationIssue
	for _, issue := range issues {
		if issue.Field == field {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}
