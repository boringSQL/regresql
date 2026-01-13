package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	checkBaselinesCwd string

	checkBaselinesCmd = &cobra.Command{
		Use:   "check-baselines",
		Short: "Check baseline-to-snapshot correlation",
		Long: `Check which snapshot version each baseline was created against.

This command shows which baselines were created with which snapshot versions,
helping identify baselines that may be out of sync with the current snapshot.

Examples:
  regresql check-baselines
  regresql check-baselines -C /path/to/project`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(checkBaselinesCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}
			if err := runCheckBaselines(); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}
)

func init() {
	RootCmd.AddCommand(checkBaselinesCmd)

	checkBaselinesCmd.Flags().StringVarP(&checkBaselinesCwd, "cwd", "C", ".", "Change to directory")
}

func runCheckBaselines() error {
	expectedDir := regresql.GetExpectedDir(checkBaselinesCwd)
	meta, err := regresql.LoadBaselineMetadata(expectedDir)
	if err != nil {
		return fmt.Errorf("failed to load baseline metadata: %w", err)
	}

	if len(meta.Baselines) == 0 {
		fmt.Println("No baseline metadata found.")
		fmt.Println("\nBaseline metadata is recorded when running 'regresql update'.")
		fmt.Println("Run 'regresql update' to record metadata for existing baselines.")
		return nil
	}

	snapshotsDir := regresql.GetSnapshotsDir(checkBaselinesCwd)
	snapshotMeta, err := regresql.ReadSnapshotMetadata(snapshotsDir)
	if err != nil {
		fmt.Println("No snapshot metadata found.")
		snapshotMeta = nil
	}

	var currentSnapshot *regresql.SnapshotInfo
	if snapshotMeta != nil {
		currentSnapshot = snapshotMeta.Current
	}

	groups := regresql.GroupBaselinesBySnapshot(meta)

	var sortedKeys []string
	for key := range groups {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	fmt.Println("Baselines by snapshot version:")
	fmt.Println()

	for _, key := range sortedKeys {
		baselines := groups[key]
		sort.Strings(baselines)

		suffix := ""
		if currentSnapshot != nil {
			for _, path := range baselines {
				if info := meta.Baselines[path]; info != nil && info.SnapshotHash == currentSnapshot.Hash {
					suffix = " [current]"
					break
				}
			}
		}

		fmt.Printf("  %s%s:\n", key, suffix)
		for _, baseline := range baselines {
			fmt.Printf("    - %s\n", baseline)
		}
		fmt.Println()
	}

	if currentSnapshot != nil {
		matched, outdated := regresql.CheckBaselineCorrelation(meta, currentSnapshot)

		if len(outdated) > 0 {
			fmt.Printf("WARNING: %d baseline(s) created with outdated snapshots.\n", len(outdated))
			fmt.Println("These may fail tests against the current snapshot.")
			fmt.Println()
			fmt.Println("Outdated baselines:")
			sort.Strings(outdated)
			for _, path := range outdated {
				info := meta.Baselines[path]
				tag := info.SnapshotTag
				if tag == "" {
					tag = regresql.TruncateHash(info.SnapshotHash)
				}
				fmt.Printf("  - %s (created with %s)\n", path, tag)
			}
			fmt.Println()
			fmt.Println("To synchronize:")
			fmt.Println("  regresql update")
			fmt.Println()
		} else if len(matched) > 0 {
			fmt.Printf("All %d baseline(s) are synchronized with the current snapshot.\n", len(matched))
		}
	} else {
		fmt.Println("Note: No current snapshot found. Cannot check for outdated baselines.")
	}

	return nil
}
