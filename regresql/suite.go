package regresql

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

/*
Suite implements a test suite, which is found in the Root directory and
contains a list of Dirs folders, each containing a list of SQL query files.
The RegressDir slot contains the directory where regresql stores its files:
the query plans with bound parameters, their expected outputs and the actual
results obtained when running `regresql test`.

Rather than handling a fully recursive data structure, which isn't necessary
for our endeavours, we maintain a fixed two-levels data structure. The
Printf() method dipatched on a Suite method is callable from the main
command and shows our structure organisation:

    $ regresql list
    .
      src/sql/
        album-by-artist.sql
        album-tracks.sql
        artist.sql
        genre-topn.sql
        genre-tracks.sql

*/
type (
	Suite struct {
		Root          string
		RegressDir    string
		Dirs          []Folder
		PlanDir       string
		ExpectedDir   string
		OutDir        string
		BaselineDir   string
		runFilter     string
		pathFilters   []string
		ignoreMatcher *IgnoreMatcher
	}

	// createExpectedOptions controls behavior of createExpectedResults
	createExpectedOptions struct {
		Commit      bool
		Pending     bool
		Interactive bool
		DryRun      bool
		Snapshot    *SnapshotInfo // Current snapshot for baseline metadata tracking
	}

	/*
		Folder implements a directory from the source repository wherein we found
		some SQL files. Folder are only implemented as part of a Suite instance.
	*/
	Folder struct {
		Dir   string
		Files []string
	}
)

// newSuite creates a new Suite instance
func newSuite(root string) *Suite {
	var folders []Folder
	regressDir := filepath.Join(root, "regresql")
	planDir := filepath.Join(root, "regresql", "plans")
	expectedDir := filepath.Join(root, "regresql", "expected")
	outDir := filepath.Join(root, "regresql", "out")
	baselineDir := filepath.Join(root, "regresql", "baselines")
	return &Suite{
		Root:        root,
		RegressDir:  regressDir,
		Dirs:        folders,
		PlanDir:     planDir,
		ExpectedDir: expectedDir,
		OutDir:      outDir,
		BaselineDir: baselineDir,
		runFilter:   "",
	}
}

// newFolder created a new Folder instance
func newFolder(path string) *Folder {
	return &Folder{path, []string{}}
}

// GetExpectedDir returns the path to the expected directory for a given root
func GetExpectedDir(root string) string {
	return filepath.Join(root, "regresql", "expected")
}

// appendPath appends a path to our Suite instance.
//
// appendPath first searches in s if we already have seen the relative
// directory of path, adding it to s if not. Then it adds the base name of
// path to the Folder.
func (s *Suite) appendPath(path string) *Suite {
	dir, _ := filepath.Rel(s.Root, filepath.Dir(path))
	var name string = filepath.Base(path)

	// search dir in folders
	for i := range s.Dirs {
		if s.Dirs[i].Dir == dir {
			// dir is already known, append file here
			s.Dirs[i].Files = append(s.Dirs[i].Files, name)
			return s
		}
	}

	// we didn't find the path folder, append a new entry and return it
	f := newFolder(dir)
	f.Files = append(f.Files, name)
	s.Dirs = append(s.Dirs, *f)
	return s
}

// Walk walks the root directory recursively in search of *.sql files and
// returns a Suite instance representing the traversal. It respects ignore
// patterns from both .regresignore file and the config's ignore field.
func Walk(root string, configIgnorePatterns []string) *Suite {
	suite := newSuite(root)

	// Load ignore patterns from .regresignore file
	ignoreMatcher, err := LoadIgnoreFile(root)
	if err != nil {
		fmt.Printf("Warning: failed to load .regresignore: %s\n", err)
		ignoreMatcher = NewIgnoreMatcher(root, []string{})
	}

	// Add patterns from config
	if len(configIgnorePatterns) > 0 {
		ignoreMatcher.patterns = append(ignoreMatcher.patterns, configIgnorePatterns...)
	}

	suite.ignoreMatcher = ignoreMatcher

	visit := func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if this path should be ignored
		if ignoreMatcher.ShouldIgnore(path, f.IsDir()) {
			if f.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process SQL files
		if !f.IsDir() && filepath.Ext(path) == ".sql" {
			suite = suite.appendPath(path)
		}
		return nil
	}
	filepath.Walk(root, visit)

	return suite
}

// SetRunFilter sets the run filter pattern for the suite
func (s *Suite) SetRunFilter(pattern string) {
	s.runFilter = pattern
}

// SetPathFilters sets the path filters for the suite
func (s *Suite) SetPathFilters(paths []string) {
	s.pathFilters = paths
}

// matchesPathFilter checks if a file path matches any of the path filters
// Returns true if there's no filter set, or if the path matches any filter
func (s *Suite) matchesPathFilter(filePath string) bool {
	if len(s.pathFilters) == 0 {
		return true
	}

	cleanPath := filepath.Clean(filePath)

	for _, pattern := range s.pathFilters {
		cleanPattern := filepath.Clean(pattern)

		// Exact match
		if cleanPath == cleanPattern {
			return true
		}

		// Directory prefix match - ensure we match on path boundaries
		// e.g., "orders" should match "orders/get.sql" but not "orders_old/get.sql"
		if strings.HasPrefix(cleanPath, cleanPattern+string(filepath.Separator)) {
			return true
		}

		// Glob pattern match
		if matched, _ := filepath.Match(cleanPattern, cleanPath); matched {
			return true
		}
	}
	return false
}

// matchesRunFilter checks if a file name or query name matches the run filter
// Returns true if there's no filter set, or if either the file name or query name matches
func (s *Suite) matchesRunFilter(fileName, queryName string) bool {
	// If no filter is set, match everything
	if s.runFilter == "" {
		return true
	}

	// Try to compile the regex pattern
	re, err := regexp.Compile(s.runFilter)
	if err != nil {
		// If the pattern is invalid, treat it as a literal string match
		return strings.Contains(fileName, s.runFilter) || strings.Contains(queryName, s.runFilter)
	}

	// Match against both file name and query name
	return re.MatchString(fileName) || re.MatchString(queryName)
}

// Println(Suite) pretty prints the Suite instance to standard out.
func (s *Suite) Println() {
	fmt.Printf("%s\n", s.Root)
	for _, folder := range s.Dirs {
		fmt.Printf("  %s/\n", folder.Dir)
		for _, name := range folder.Files {
			fmt.Printf("    %s\n", name)
		}
	}
}

// initRegressHierarchy walks a Suite instance s and creates the regresql
// plans directories for the queries found in s, copying the directory
// structure in its own space.
func (s *Suite) initRegressHierarchy() error {
	for _, folder := range s.Dirs {
		rdir := filepath.Join(s.PlanDir, folder.Dir)

		if err := maybeMkdirAll(rdir); err != nil {
			return fmt.Errorf("Failed to create test plans directory: %s", err)
		}

		for _, name := range folder.Files {
			qfile := filepath.Join(s.Root, folder.Dir, name)

			queries, err := parseQueryFile(qfile)
			if err != nil {
				return err
			}

			for _, q := range queries {
				// Skip if the query doesn't match the run filter
				if !s.matchesRunFilter(name, q.Name) {
					continue
				}

				// Skip queries with notest option
				opts := q.GetRegressQLOptions()
				if opts.NoTest {
					fmt.Printf("Skipping query '%s' (notest)\n", q.Name)
					continue
				}

				if _, err := q.CreateEmptyPlan(rdir); err != nil {
					fmt.Println("Skipping:", err)
				}
			}
		}
	}
	return nil
}

// createExpectedResults walks the s Suite instance and runs its queries,
// storing the results in the expected files.
// Each query runs in its own transaction that rolls back (unless commit is true).
func (s *Suite) createExpectedResults(pguri string, opts createExpectedOptions) error {
	db, err := sql.Open("pgx", pguri)
	if err != nil {
		return fmt.Errorf("Failed to connect to '%s': %s\n", pguri, err)
	}
	defer db.Close()

	var dryRunSummary []string
	var prompter *InteractivePrompter
	if opts.Interactive {
		prompter = NewInteractivePrompter()
	}

	if !opts.DryRun {
		fmt.Println("Writing expected Result Sets:")
	}

	for _, folder := range s.Dirs {
		rdir := filepath.Join(s.PlanDir, folder.Dir)
		edir := filepath.Join(s.ExpectedDir, folder.Dir)

		// Build relative path for filtering
		relPath := folder.Dir

		// Check path filter at folder level
		if !s.matchesPathFilter(relPath) && !s.matchesPathFilter(relPath+"/") {
			continue
		}

		if !opts.DryRun {
			maybeMkdirAll(edir)
			fmt.Printf("  %s\n", edir)
		}

		for _, name := range folder.Files {
			qfile := filepath.Join(s.Root, folder.Dir, name)
			queryPath := filepath.Join(relPath, name)

			// Check path filter at file level
			if !s.matchesPathFilter(queryPath) {
				continue
			}

			queries, err := parseQueryFile(qfile)
			if err != nil {
				return err
			}

			for _, q := range queries {
				if !s.matchesRunFilter(name, q.Name) {
					continue
				}
				if qopts := q.GetRegressQLOptions(); qopts.NoTest {
					continue
				}

				// Check for pending-only mode
				if opts.Pending {
					expectedPath := filepath.Join(edir, q.Name+".json")
					if fileExists(expectedPath) {
						continue // Skip - already has baseline
					}
				}

				p, err := q.GetPlan(rdir)
				if err != nil {
					return err
				}

				// Execute query and handle results
				var writtenFiles []string
				if err := s.runInTransaction(db, opts.Commit, func(tx *sql.Tx) error {
					if err := p.Execute(tx); err != nil {
						return err
					}

					// Dry-run: just record what would be updated
					if opts.DryRun {
						for _, rs := range p.ResultSets {
							dryRunSummary = append(dryRunSummary, fmt.Sprintf("  Would update: %s", filepath.Base(rs.Filename)))
						}
						return nil
					}

					// Interactive mode: compute diff and prompt for each change
					if opts.Interactive {
						diff := p.ComputeDiffForInteractive(edir)
						action := prompter.PromptAccept(q.Name, diff)
						switch action {
						case "skip":
							return nil
						case "quit":
							return ErrUserQuit
						}
					}

					if err := p.WriteResultSets(edir); err != nil {
						return err
					}

					// Track written files for metadata recording
					for _, rs := range p.ResultSets {
						writtenFiles = append(writtenFiles, filepath.Join(edir, filepath.Base(rs.Filename)))
					}
					return nil
				}); err != nil {
					if err == ErrUserQuit {
						fmt.Println("\nUpdate cancelled by user")
						return nil
					}
					return err
				}

				// Record baseline metadata for written files
				if !opts.DryRun && opts.Snapshot != nil {
					for _, baselinePath := range writtenFiles {
						if err := RecordBaselineUpdate(s.ExpectedDir, baselinePath, opts.Snapshot, ""); err != nil {
							fmt.Printf("Warning: failed to record baseline metadata: %s\n", err)
						}
					}
				}

				if !opts.DryRun {
					for _, rs := range p.ResultSets {
						fmt.Printf("    %s\n", filepath.Base(rs.Filename))
					}
				}
			}
		}
	}

	// Print dry-run summary
	if opts.DryRun {
		if len(dryRunSummary) == 0 {
			fmt.Println("Dry run: no changes to make")
		} else {
			fmt.Println("Dry run - no files modified:")
			for _, line := range dryRunSummary {
				fmt.Println(line)
			}
		}
	}

	return nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// testQueries walks the s Suite instance and runs queries against the plans
// and stores results in the out directory for manual inspection if
// necessary. It then compares the actual output to the expected output and
// reports results using the specified formatter.
// Each query runs in its own transaction that rolls back (unless commit is true).
// Returns the test summary for exit code determination.
func (s *Suite) testQueries(pguri string, formatter OutputFormatter, outputPath string, commit bool) (*TestSummary, error) {
	db, err := sql.Open("pgx", pguri)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to '%s': %s\n", pguri, err)
	}
	defer db.Close()

	w, close, err := getWriter(outputPath)
	if err != nil {
		return nil, err
	}
	defer close()

	summary := NewTestSummary()
	if err := formatter.Start(w); err != nil {
		return nil, err
	}

	for _, folder := range s.Dirs {
		rdir := filepath.Join(s.PlanDir, folder.Dir)
		edir := filepath.Join(s.ExpectedDir, folder.Dir)
		odir := filepath.Join(s.OutDir, folder.Dir)
		bdir := filepath.Join(s.BaselineDir, folder.Dir)
		maybeMkdirAll(odir)

		for _, name := range folder.Files {
			qfile := filepath.Join(s.Root, folder.Dir, name)

			queries, err := parseQueryFile(qfile)
			if err != nil {
				return nil, err
			}

			for _, q := range queries {
				if !s.matchesRunFilter(name, q.Name) {
					continue
				}

				opts := q.GetRegressQLOptions()
				if opts.NoTest {
					continue
				}

				p, err := q.GetPlan(rdir)
				if err != nil {
					return nil, err
				}

				if err := s.runInTransaction(db, commit, func(tx *sql.Tx) error {
					if err := p.Execute(tx); err != nil {
						return err
					}
					if err := p.WriteResultSets(odir); err != nil {
						return err
					}

					for _, r := range p.CompareResultSetsToResults(s.RegressDir, edir) {
						summary.AddResult(r)
						if err := formatter.AddResult(r, w); err != nil {
							return err
						}
					}

					if !opts.NoBaseline && hasBaselines(p.Query, bdir, p.Names) {
						for _, r := range p.CompareBaselinesToResults(bdir, tx, DefaultCostThresholdPercent) {
							summary.AddResult(r)
							if err := formatter.AddResult(r, w); err != nil {
								return err
							}
						}
					}
					return nil
				}); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := formatter.Finish(summary, w); err != nil {
		return nil, err
	}
	return summary, nil
}

// runInTransaction executes fn within a transaction, rolling back on error or if commit is false
func (s *Suite) runInTransaction(db *sql.DB, commit bool, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	if commit {
		return tx.Commit()
	}
	return tx.Rollback()
}

// executeAllQueries executes all queries and saves results to outputDir.
// Used by migrate command to capture before/after states.
// Queries with parameters but no plan files are skipped with a warning.
func (s *Suite) executeAllQueries(pguri, outputDir string, verbose bool) (int, error) {
	db, err := sql.Open("pgx", pguri)
	if err != nil {
		return 0, fmt.Errorf("failed to connect to '%s': %w", pguri, err)
	}
	defer db.Close()

	count := 0
	skipped := 0

	for _, folder := range s.Dirs {
		rdir := filepath.Join(s.PlanDir, folder.Dir)
		odir := filepath.Join(outputDir, folder.Dir)

		for _, name := range folder.Files {
			qfile := filepath.Join(s.Root, folder.Dir, name)

			queries, err := parseQueryFile(qfile)
			if err != nil {
				return count, err
			}

			for _, q := range queries {
				opts := q.GetRegressQLOptions()
				if opts.NoTest {
					continue
				}

				p, err := q.GetPlan(rdir)
				if err != nil {
					// Skip queries that require plans but don't have them
					if verbose {
						fmt.Printf("  [skip] %s/%s: %v\n", folder.Dir, name, err)
					}
					skipped++
					continue
				}

				// Only create output directory if we have queries to run
				maybeMkdirAll(odir)

				if err := s.runInTransaction(db, false, func(tx *sql.Tx) error {
					if err := p.Execute(tx); err != nil {
						return err
					}
					if err := p.WriteResultSets(odir); err != nil {
						return err
					}
					return nil
				}); err != nil {
					return count, err
				}

				count += len(p.ResultSets)
				if verbose {
					fmt.Printf("  %s/%s (%d bindings)\n", folder.Dir, name, len(p.ResultSets))
				}
			}
		}
	}

	if skipped > 0 && !verbose {
		fmt.Printf("  (skipped %d queries without plans - use --verbose to see details)\n", skipped)
	}

	return count, nil
}

// Only create dir(s) when it doesn't exists already
func maybeMkdirAll(dir string) error {
	stat, err := os.Stat(dir)
	if err != nil || !stat.IsDir() {
		fmt.Printf("Creating directory '%s'\n", dir)

		err := os.MkdirAll(dir, 0755)

		if err != nil {
			return err
		}
	}
	return nil
}

// hasBaselines checks if any baseline files exist for the given query
func hasBaselines(q *Query, baselineDir string, names []string) bool {
	// If query has no parameters, check for single baseline file
	if len(names) == 0 {
		baselinePath := getBaselinePath(q, baselineDir, "")
		if _, err := os.Stat(baselinePath); err == nil {
			return true
		}
		return false
	}

	// For parameterized queries, check if at least one binding has a baseline
	for _, name := range names {
		baselinePath := getBaselinePath(q, baselineDir, name)
		if _, err := os.Stat(baselinePath); err == nil {
			return true
		}
	}
	return false
}
