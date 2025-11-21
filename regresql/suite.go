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
		ignoreMatcher *IgnoreMatcher
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
func (s *Suite) createExpectedResults(pguri string, useFixtures bool) error {
	db, err := sql.Open("pgx", pguri)

	if err != nil {
		return fmt.Errorf("Failed to connect to '%s': %s\n", pguri, err)
	}
	defer db.Close()

	var fixtureManager *FixtureManager
	if useFixtures {
		fixtureManager, err = NewFixtureManager(s.Root, db)
		if err != nil {
			return fmt.Errorf("Failed to create fixture manager: %w", err)
		}
		if err := fixtureManager.IntrospectSchema(); err != nil {
			return fmt.Errorf("Failed to introspect schema: %w", err)
		}
	}

	fmt.Println("Writing expected Result Sets:")

	for _, folder := range s.Dirs {
		rdir := filepath.Join(s.PlanDir, folder.Dir)
		edir := filepath.Join(s.ExpectedDir, folder.Dir)
		maybeMkdirAll(edir)

		fmt.Printf("  %s\n", edir)

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
					continue
				}

				p, err := q.GetPlan(rdir)
				if err != nil {
					return err
				}

				// Apply fixtures if configured
				if useFixtures && len(p.Fixtures) > 0 {
					if err := fixtureManager.BeginTransaction(); err != nil {
						return fmt.Errorf("Failed to begin transaction for fixtures: %w", err)
					}
					if err := fixtureManager.ApplyFixtures(p.Fixtures); err != nil {
						fixtureManager.Rollback()
						return fmt.Errorf("Failed to apply fixtures: %w", err)
					}
				}

				if err := p.Execute(db); err != nil {
					if useFixtures && len(p.Fixtures) > 0 {
						fixtureManager.Rollback()
					}
					return err
				}

				if err := p.WriteResultSets(edir); err != nil {
					if useFixtures && len(p.Fixtures) > 0 {
						fixtureManager.Rollback()
					}
					return err
				}

				// Cleanup fixtures
				if useFixtures && len(p.Fixtures) > 0 {
					fixture, _ := fixtureManager.LoadFixture(p.Fixtures[0])
					if p.Cleanup != "" {
						fixture.Cleanup = p.Cleanup
					}
					if err := fixtureManager.Cleanup(fixture); err != nil {
						return fmt.Errorf("Failed to cleanup fixtures: %w", err)
					}
				}

				for _, rs := range p.ResultSets {
					fmt.Printf("    %s\n", filepath.Base(rs.Filename))
				}
			}
		}
	}
	return nil
}

// testQueries walks the s Suite instance and runs queries against the plans
// and stores results in the out directory for manual inspection if
// necessary. It then compares the actual output to the expected output and
// reports results using the specified formatter.
func (s *Suite) testQueries(pguri string, formatter OutputFormatter, outputPath string) error {
	db, err := sql.Open("pgx", pguri)
	if err != nil {
		return fmt.Errorf("Failed to connect to '%s': %s\n", pguri, err)
	}
	defer db.Close()

	fixtureManager, err := NewFixtureManager(s.Root, db)
	if err != nil {
		return fmt.Errorf("Failed to create fixture manager: %w", err)
	}

	if err := fixtureManager.IntrospectSchema(); err != nil {
		return fmt.Errorf("Failed to introspect schema: %w", err)
	}

	w, close, err := getWriter(outputPath)
	if err != nil {
		return err
	}
	defer close()

	summary := NewTestSummary()
	if err := formatter.Start(w); err != nil {
		return err
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
				return err
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
					return err
				}

				if len(p.Fixtures) > 0 {
					if err := fixtureManager.BeginTransaction(); err != nil {
						return fmt.Errorf("Failed to begin transaction for fixtures: %w", err)
					}

					if err := fixtureManager.ApplyFixtures(p.Fixtures); err != nil {
						fixtureManager.Rollback()
						return fmt.Errorf("Failed to apply fixtures: %w", err)
					}
				}

				if err := p.Execute(db); err != nil {
					if len(p.Fixtures) > 0 {
						fixtureManager.Rollback()
					}
					return err
				}

				if err := p.WriteResultSets(odir); err != nil {
					if len(p.Fixtures) > 0 {
						fixtureManager.Rollback()
					}
					return err
				}

				for _, r := range p.CompareResultSetsToResults(s.RegressDir, edir) {
					summary.AddResult(r)
					if err := formatter.AddResult(r, w); err != nil {
						if len(p.Fixtures) > 0 {
							fixtureManager.Rollback()
						}
						return err
					}
				}

				if !opts.NoBaseline && hasBaselines(p.Query, bdir, p.Names) {
					for _, r := range p.CompareBaselinesToResults(bdir, db, DefaultCostThresholdPercent) {
						summary.AddResult(r)
						if err := formatter.AddResult(r, w); err != nil {
							if len(p.Fixtures) > 0 {
								fixtureManager.Rollback()
							}
							return err
						}
					}
				}

				if len(p.Fixtures) > 0 {
					fixture, _ := fixtureManager.LoadFixture(p.Fixtures[0])
					if p.Cleanup != "" {
						fixture.Cleanup = p.Cleanup
					}
					if err := fixtureManager.Cleanup(fixture); err != nil {
						return fmt.Errorf("Failed to cleanup fixtures: %w", err)
					}
				}
			}
		}
	}

	return formatter.Finish(summary, w)
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
