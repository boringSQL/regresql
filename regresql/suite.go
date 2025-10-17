package regresql

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/lib/pq"
	"github.com/mndrix/tap-go"
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
type Suite struct {
	Root        string
	RegressDir  string
	Dirs        []Folder
	PlanDir     string
	ExpectedDir string
	OutDir      string
	BaselineDir string
}

/*
Folder implements a directory from the source repository wherein we found
some SQL files. Folder are only implemented as part of a Suite instance.
*/
type Folder struct {
	Dir   string
	Files []string
}

// newSuite creates a new Suite instance
func newSuite(root string) *Suite {
	var folders []Folder
	regressDir := filepath.Join(root, "regresql")
	planDir := filepath.Join(root, "regresql", "plans")
	expectedDir := filepath.Join(root, "regresql", "expected")
	outDir := filepath.Join(root, "regresql", "out")
	baselineDir := filepath.Join(root, "regresql", "baselines")
	return &Suite{root, regressDir, folders, planDir, expectedDir, outDir, baselineDir}
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
// returns a Suite instance representing the traversal.
func Walk(root string) *Suite {
	suite := newSuite(root)

	visit := func(path string, f os.FileInfo, err error) error {
		if filepath.Ext(path) == ".sql" {
			suite = suite.appendPath(path)
		}
		return nil
	}
	filepath.Walk(root, visit)

	return suite
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
func (s *Suite) createExpectedResults(pguri string) error {
	db, err := sql.Open("postgres", pguri)

	if err != nil {
		return fmt.Errorf("Failed to connect to '%s': %s\n", pguri, err)
	}
	defer db.Close()

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
				// Skip queries with notest option
				opts := q.GetRegressQLOptions()
				if opts.NoTest {
					continue
				}

				p, err := q.GetPlan(rdir)
				if err != nil {
					return err
				}
				p.Execute(db)
				p.WriteResultSets(edir)

				for _, rs := range p.ResultSets {
					fmt.Printf("    %s\n", filepath.Base(rs.Filename))
				}
			}
		}
	}
	return nil
}

// testQueries walks the s Suite instance and runs queries against the plans
// and sotores results in the out directory for manual inspection if
// necessary, It then compares the actual output to the expected output and
// reports a TAP output.
func (s *Suite) testQueries(pguri string) error {
	db, err := sql.Open("postgres", pguri)

	if err != nil {
		return fmt.Errorf("Failed to connect to '%s': %s\n", pguri, err)
	}
	defer db.Close()

	t := tap.New()
	t.Header(0)

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
				// Skip queries with notest option
				opts := q.GetRegressQLOptions()
				if opts.NoTest {
					continue
				}

				p, err := q.GetPlan(rdir)
				if err != nil {
					return err
				}
				if err := p.Execute(db); err != nil {
					return err
				}
				if err := p.WriteResultSets(odir); err != nil {
					return err
				}
				p.CompareResultSets(s.RegressDir, edir, t)

				// Compare against baselines with default threshold (unless nobaseline)
				if !opts.NoBaseline {
					p.CompareBaselines(bdir, db, t, DefaultCostThresholdPercent)
				}
			}
		}
	}
	return nil
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
