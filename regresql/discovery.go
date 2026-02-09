package regresql

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type (
	DiscoveryResult struct {
		RelPath      string
		Queries      []QueryStatus
		TotalQueries int
		AddedQueries int
	}

	QueryStatus struct {
		Name     string
		HasPlan  bool
		PlanPath string
	}

	DiscoverOptions struct {
		Root       string
		ShowDetail bool
		NewOnly    bool
	}

	AddOptions struct {
		Root  string
		Paths []string
		Force bool
	}

	RemoveOptions struct {
		Root   string
		Paths  []string
		Clean  bool
		DryRun bool
	}
)

// Status returns the status indicator for display
func (d *DiscoveryResult) Status() string {
	if d.AddedQueries == d.TotalQueries {
		return "[+]"
	}
	if d.AddedQueries == 0 {
		return "[ ]"
	}
	return "[~]"
}

// StatusDetail returns detail about queries added
func (d *DiscoveryResult) StatusDetail() string {
	if d.TotalQueries == 1 {
		return "(1 query)"
	}
	if d.AddedQueries == d.TotalQueries {
		return fmt.Sprintf("(%d queries)", d.TotalQueries)
	}
	if d.AddedQueries == 0 {
		return fmt.Sprintf("(%d queries)", d.TotalQueries)
	}
	return fmt.Sprintf("(%d/%d queries added)", d.AddedQueries, d.TotalQueries)
}

// Discover walks the codebase and returns the test status of each SQL file
func Discover(opts DiscoverOptions) ([]DiscoveryResult, error) {
	config, err := ReadConfig(opts.Root)
	ignorePatterns := []string{}
	if err == nil {
		ignorePatterns = config.Ignore
	}

	suite := Walk(opts.Root, ignorePatterns)

	var results []DiscoveryResult

	for _, folder := range suite.Dirs {
		planDir := filepath.Join(suite.PlanDir, folder.Dir)

		for _, name := range folder.Files {
			qfile := filepath.Join(suite.Root, folder.Dir, name)
			relPath := filepath.Join(folder.Dir, name)

			queries, err := parseQueryFile(qfile)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", relPath, err)
			}

			result := DiscoveryResult{
				RelPath:      relPath,
				TotalQueries: len(queries),
			}

			for qname, q := range queries {
				opts := q.GetRegressQLOptions()
				if opts.NoTest {
					result.TotalQueries--
					continue
				}

				planPath := getPlanPath(q, planDir)
				planExists := hasPlan(planPath)

				qs := QueryStatus{
					Name:     qname,
					HasPlan:  planExists,
					PlanPath: planPath,
				}
				result.Queries = append(result.Queries, qs)

				if planExists {
					result.AddedQueries++
				}
			}

			// Sort queries by name for consistent output
			sort.Slice(result.Queries, func(i, j int) bool {
				return result.Queries[i].Name < result.Queries[j].Name
			})

			results = append(results, result)
		}
	}

	// Sort results by path
	sort.Slice(results, func(i, j int) bool {
		return results[i].RelPath < results[j].RelPath
	})

	return results, nil
}

// hasPlan returns true if a plan file exists for the query
func hasPlan(planPath string) bool {
	_, err := os.Stat(planPath)
	return err == nil
}

// PrintDiscoveryResults prints the discovery results to stdout
func PrintDiscoveryResults(results []DiscoveryResult, showDetail bool, newOnly ...bool) {
	var added, notAdded, partial int
	skipTracked := len(newOnly) > 0 && newOnly[0]

	fmt.Println("SQL files in project:")

	for _, r := range results {
		if r.AddedQueries == r.TotalQueries {
			added++
		} else if r.AddedQueries == 0 {
			notAdded++
		} else {
			partial++
		}

		if skipTracked && r.AddedQueries == r.TotalQueries {
			continue
		}

		fmt.Printf("  %s %s %s\n", r.Status(), r.RelPath, r.StatusDetail())

		if showDetail && len(r.Queries) > 1 {
			for _, q := range r.Queries {
				status := "+"
				if !q.HasPlan {
					status = " "
				}
				fmt.Printf("      [%s] %s\n", status, q.Name)
			}
		}
	}

	fmt.Println()
	fmt.Printf("Summary: %d added, %d not added, %d partial\n", added, notAdded, partial)
}

// AddQueries adds SQL files to the test suite by creating plan files
func AddQueries(opts AddOptions) error {
	config, err := ReadConfig(opts.Root)
	ignorePatterns := []string{}
	if err == nil {
		ignorePatterns = config.Ignore
	}

	suite := Walk(opts.Root, ignorePatterns)

	// Expand paths to actual SQL files
	sqlFiles, err := expandPaths(opts.Root, opts.Paths, suite)
	if err != nil {
		return err
	}

	if len(sqlFiles) == 0 {
		return fmt.Errorf("no SQL files found matching the specified paths")
	}

	// Make root absolute for filepath.Rel to work with absolute sqlFile paths
	absRoot, err := filepath.Abs(opts.Root)
	if err != nil {
		return fmt.Errorf("failed to resolve root path: %w", err)
	}

	var addedCount, skippedCount int

	for _, sqlFile := range sqlFiles {
		relPath, _ := filepath.Rel(absRoot, sqlFile)
		folderDir := filepath.Dir(relPath)
		planDir := filepath.Join(suite.PlanDir, folderDir)

		queries, err := parseQueryFile(sqlFile)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", relPath, err)
		}

		for _, q := range queries {
			qopts := q.GetRegressQLOptions()
			if qopts.NoTest {
				continue
			}

			planPath := getPlanPath(q, planDir)

			// Check if plan already exists
			if _, err := os.Stat(planPath); err == nil {
				if !opts.Force {
					skippedCount++
					continue
				}
				// Remove existing plan if force is enabled
				if err := os.Remove(planPath); err != nil {
					return fmt.Errorf("failed to remove existing plan %s: %w", planPath, err)
				}
			}

			// Create plan directory if needed
			if err := ensureDir(planDir); err != nil {
				return fmt.Errorf("failed to create plan directory: %w", err)
			}

			// Create empty plan
			_, err := q.CreateEmptyPlan(planDir)
			if err != nil && !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("failed to create plan for %s: %w", q.Name, err)
			}
			addedCount++
		}
	}

	fmt.Printf("\nAdded %d plan files", addedCount)
	if skippedCount > 0 {
		fmt.Printf(" (skipped %d existing, use --force to overwrite)", skippedCount)
	}
	fmt.Println()

	if addedCount > 0 {
		fmt.Println("\nNext steps:")
		fmt.Println("  1. Edit plan files to add parameter values")
		fmt.Println("  2. Run 'regresql update' to generate expected outputs")
	}

	return nil
}

// RemoveQueries removes SQL files from the test suite
func RemoveQueries(opts RemoveOptions) error {
	config, err := ReadConfig(opts.Root)
	ignorePatterns := []string{}
	if err == nil {
		ignorePatterns = config.Ignore
	}

	suite := Walk(opts.Root, ignorePatterns)

	// Expand paths to actual SQL files
	sqlFiles, err := expandPaths(opts.Root, opts.Paths, suite)
	if err != nil {
		return err
	}

	if len(sqlFiles) == 0 {
		return fmt.Errorf("no SQL files found matching the specified paths")
	}

	absRoot, err := filepath.Abs(opts.Root)
	if err != nil {
		return fmt.Errorf("failed to resolve root path: %w", err)
	}

	var filesToDelete []string

	for _, sqlFile := range sqlFiles {
		relPath, _ := filepath.Rel(absRoot, sqlFile)
		folderDir := filepath.Dir(relPath)
		planDir := filepath.Join(suite.PlanDir, folderDir)
		expectedDir := filepath.Join(suite.ExpectedDir, folderDir)
		baselineDir := filepath.Join(suite.BaselineDir, folderDir)

		queries, err := parseQueryFile(sqlFile)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", relPath, err)
		}

		for _, q := range queries {
			// Plan file
			planPath := getPlanPath(q, planDir)
			if _, err := os.Stat(planPath); err == nil {
				filesToDelete = append(filesToDelete, planPath)
			}

			if opts.Clean {
				// Expected result files
				expectedPattern := getResultSetPathPattern(q, expectedDir)
				matches, _ := filepath.Glob(expectedPattern)
				filesToDelete = append(filesToDelete, matches...)

				// Baseline files
				baselinePattern := getBaselinePathPattern(q, baselineDir)
				matches, _ = filepath.Glob(baselinePattern)
				filesToDelete = append(filesToDelete, matches...)
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, f := range filesToDelete {
		if !seen[f] {
			seen[f] = true
			unique = append(unique, f)
		}
	}
	filesToDelete = unique

	if len(filesToDelete) == 0 {
		fmt.Println("No files to remove.")
		return nil
	}

	// Sort for consistent output
	sort.Strings(filesToDelete)

	if opts.DryRun {
		fmt.Println("Would delete the following files:")
		for _, f := range filesToDelete {
			fmt.Printf("  %s\n", f)
		}
		return nil
	}

	// Delete files
	var deleted int
	for _, f := range filesToDelete {
		if err := os.Remove(f); err != nil {
			fmt.Printf("Warning: failed to delete %s: %s\n", f, err)
			continue
		}
		fmt.Printf("  Deleted: %s\n", f)
		deleted++
	}

	fmt.Printf("\nRemoved %d files\n", deleted)

	return nil
}

// expandPaths expands the given paths to actual SQL file paths
func expandPaths(root string, paths []string, suite *Suite) ([]string, error) {
	var result []string
	seen := make(map[string]bool)

	// Make root absolute for consistent path comparison
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root path: %w", err)
	}

	// Build a set of all SQL files in the suite (with absolute paths)
	allSQLFiles := make(map[string]bool)
	for _, folder := range suite.Dirs {
		for _, name := range folder.Files {
			fullPath := filepath.Join(absRoot, folder.Dir, name)
			allSQLFiles[fullPath] = true
		}
	}

	for _, p := range paths {
		// Make path absolute
		absPath, err := filepath.Abs(filepath.Join(root, p))
		if err != nil {
			continue
		}

		// Check if it's a directory
		info, err := os.Stat(absPath)
		if err == nil && info.IsDir() {
			// Add all SQL files in this directory (recursively)
			for sqlFile := range allSQLFiles {
				// Match files in this directory or subdirectories
				if strings.HasPrefix(sqlFile, absPath+string(filepath.Separator)) {
					if !seen[sqlFile] {
						seen[sqlFile] = true
						result = append(result, sqlFile)
					}
				}
				// Also match files directly in directory (check parent)
				if filepath.Dir(sqlFile) == absPath {
					if !seen[sqlFile] {
						seen[sqlFile] = true
						result = append(result, sqlFile)
					}
				}
			}
			continue
		}

		// Try as exact file path (accept any .sql file that exists on disk)
		if strings.HasSuffix(absPath, ".sql") && fileExists(absPath) {
			if !seen[absPath] {
				seen[absPath] = true
				result = append(result, absPath)
			}
			continue
		}

		// Try glob pattern
		matches, err := filepath.Glob(absPath)
		if err == nil && len(matches) > 0 {
			for _, m := range matches {
				absMatch, _ := filepath.Abs(m)
				if strings.HasSuffix(absMatch, ".sql") && allSQLFiles[absMatch] {
					if !seen[absMatch] {
						seen[absMatch] = true
						result = append(result, absMatch)
					}
				}
			}
			continue
		}
	}

	sort.Strings(result)
	return result, nil
}

// getResultSetPathPattern returns a glob pattern for expected result files
func getResultSetPathPattern(q *Query, expectedDir string) string {
	basename := strings.TrimSuffix(filepath.Base(q.Path), filepath.Ext(q.Path))
	// If query name matches file basename, don't duplicate it
	if q.Name == basename {
		return filepath.Join(expectedDir, basename+"*.json")
	}
	return filepath.Join(expectedDir, basename+"_"+q.Name+"*.json")
}

// getBaselinePathPattern returns a glob pattern for baseline files
func getBaselinePathPattern(q *Query, baselineDir string) string {
	basename := strings.TrimSuffix(filepath.Base(q.Path), filepath.Ext(q.Path))
	// If query name matches file basename, don't duplicate it
	if q.Name == basename {
		return filepath.Join(baselineDir, basename+"*.json")
	}
	return filepath.Join(baselineDir, basename+"_"+q.Name+"*.json")
}
