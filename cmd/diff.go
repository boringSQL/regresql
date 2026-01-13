package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	diffCwd       string
	diffFrom      string
	diffTo        string
	diffQuery     string
	diffRunFilter string

	diffCmd = &cobra.Command{
		Use:   "diff",
		Short: "Compare query outputs between two snapshot versions",
		Long: `Compare query outputs between two snapshot versions.

This command restores two different snapshots and runs all queries against both,
then shows the differences in query output.

Examples:
  regresql diff --from v1 --to v2
  regresql diff --from v1 --to v3 --query orders/get_order_total.sql
  regresql diff --from sha256:abc123 --to current`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(diffCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			if err := runDiff(); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}
)

func init() {
	RootCmd.AddCommand(diffCmd)

	diffCmd.Flags().StringVarP(&diffCwd, "cwd", "C", ".", "Change to directory")
	diffCmd.Flags().StringVar(&diffFrom, "from", "", "Source snapshot (tag or hash prefix)")
	diffCmd.Flags().StringVar(&diffTo, "to", "", "Target snapshot (tag, hash, or 'current', default: current)")
	diffCmd.Flags().StringVar(&diffQuery, "query", "", "Specific query to compare (optional)")
	diffCmd.Flags().StringVar(&diffRunFilter, "run", "", "Run only queries matching regexp")

	diffCmd.MarkFlagRequired("from")
}

func runDiff() error {
	snapshotsDir := regresql.GetSnapshotsDir(diffCwd)

	metadata, err := regresql.ReadSnapshotMetadata(snapshotsDir)
	if err != nil {
		return fmt.Errorf("no snapshot metadata found: %w", err)
	}

	fromInfo, err := regresql.ResolveSnapshot(metadata, diffFrom)
	if err != nil {
		return fmt.Errorf("cannot resolve --from snapshot: %w", err)
	}

	toRef := diffTo
	if toRef == "" || toRef == "current" {
		if metadata.Current == nil {
			return fmt.Errorf("no current snapshot")
		}
		toRef = metadata.Current.Hash
	}
	toInfo, err := regresql.ResolveSnapshot(metadata, toRef)
	if err != nil {
		return fmt.Errorf("cannot resolve --to snapshot: %w", err)
	}

	if !regresql.SnapshotExists(fromInfo) {
		return fmt.Errorf("source snapshot file not found: %s", fromInfo.Path)
	}
	if !regresql.SnapshotExists(toInfo) {
		return fmt.Errorf("target snapshot file not found: %s", toInfo.Path)
	}

	if fromInfo.Hash == toInfo.Hash {
		fmt.Printf("Both snapshots are identical (%s)\n", regresql.FormatSnapshotRef(fromInfo))
		return nil
	}

	fmt.Printf("Comparing snapshots:\n")
	fmt.Printf("  From: %s (%s)\n", regresql.FormatSnapshotRef(fromInfo), fromInfo.Path)
	fmt.Printf("  To:   %s (%s)\n", regresql.FormatSnapshotRef(toInfo), toInfo.Path)
	fmt.Println()

	result, err := regresql.DiffSnapshots(diffCwd, fromInfo, toInfo, diffQuery, diffRunFilter)
	if err != nil {
		return err
	}

	printDiffResult(result, fromInfo, toInfo)

	return nil
}

func printDiffResult(result *regresql.SnapshotDiffResult, from, to *regresql.SnapshotInfo) {
	if len(result.Changed) == 0 && len(result.Errors) == 0 {
		fmt.Printf("No differences found (%d queries compared)\n", len(result.Unchanged))
		return
	}

	if len(result.Changed) > 0 {
		fmt.Printf("CHANGED (%d):\n", len(result.Changed))
		for _, diff := range result.Changed {
			fmt.Printf("  %s\n", diff.QueryPath)
			if diff.FromRows != diff.ToRows {
				fmt.Printf("    Rows: %d → %d\n", diff.FromRows, diff.ToRows)
			}
			if diff.Diff != "" {
				// Show first few lines of diff
				lines := splitLines(diff.Diff, 5)
				for _, line := range lines {
					fmt.Printf("    %s\n", line)
				}
				if len(lines) == 5 {
					fmt.Printf("    ...\n")
				}
			}
		}
		fmt.Println()
	}

	if len(result.Errors) > 0 {
		fmt.Printf("ERRORS (%d):\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("  %s: %s\n", e.QueryPath, e.Error)
		}
		fmt.Println()
	}

	fmt.Printf("SUMMARY:\n")
	fmt.Printf("  Changed:   %d\n", len(result.Changed))
	fmt.Printf("  Unchanged: %d\n", len(result.Unchanged))
	if len(result.Errors) > 0 {
		fmt.Printf("  Errors:    %d\n", len(result.Errors))
	}
}

func splitLines(s string, max int) []string {
	var lines []string
	start := 0
	count := 0
	for i := 0; i < len(s) && count < max; i++ {
		if s[i] == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
				count++
			}
			start = i + 1
		}
	}
	if start < len(s) && count < max {
		lines = append(lines, s[start:])
	}
	return lines
}
