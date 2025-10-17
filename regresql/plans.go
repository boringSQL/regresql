package regresql

import (
	"bytes"
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	_ "github.com/lib/pq"
	"github.com/theherk/viper" // fork with write support
)

/*
A query plan associates a Query parsed from a Path (name of the file on
disk) and a list of set of parameters used to run the query. Each set of
parameters as a name in Names[i] and a list of bindings in Bindings[i]. When
the query is executed we store its output in ResultSets[i].
*/
type Plan struct {
	Query      *Query
	Path       string // the file path where we read the Plan from
	Names      []string
	Bindings   []map[string]string
	ResultSets []ResultSet
}

// CreateEmptyPlan creates a YAML file where to store the set of parameters
// associated with a query.
func (q *Query) CreateEmptyPlan(dir string) (*Plan, error) {
	var names []string
	var bindings []map[string]string
	pfile := getPlanPath(q, dir)

	if _, err := os.Stat(pfile); !os.IsNotExist(err) {
		var p Plan
		return &p, fmt.Errorf("Plan file '%s' already exists", pfile)
	}

	if len(q.NamedArgs) > 0 {
		names = make([]string, 1)
		bindings = make([]map[string]string, 1)

		names[0] = "1"
		bindings[0] = make(map[string]string)
		for _, namedArg := range q.NamedArgs {
			bindings[0][namedArg.Name] = ""
		}
	} else {
		names = []string{}
		bindings = []map[string]string{}
	}

	plan := &Plan{q, pfile, names, bindings, []ResultSet{}}
	plan.Write()

	return plan, nil
}

// GetPlan instanciates a Plan from a Query, parsing a set of actual
// parameters when it exists.
func (q *Query) GetPlan(planDir string) (*Plan, error) {
	var plan *Plan
	pfile := getPlanPath(q, planDir)

	if _, err := os.Stat(pfile); os.IsNotExist(err) {
		if len(q.Args) == 0 {
			// no Params, no Plan file, it's good
			return &Plan{q, pfile,
				[]string{},
				[]map[string]string{},
				[]ResultSet{}}, nil
		}
		e := fmt.Errorf("Failed to get plan '%s': %s\n", pfile, err)
		return plan, e
	}

	v := viper.New()
	v.SetConfigType("yaml")

	data, err := ioutil.ReadFile(pfile)

	if err != nil {
		e := fmt.Errorf("Failed to read file '%s': %s\n", pfile, err)
		return plan, e
	}

	v.ReadConfig(bytes.NewBuffer(data))

	// turns out Viper doesn't offer an easy way to build our Plan
	// Bindings from the YAML file we produced, so do it the rather
	// manual way.
	//
	// The viper.GetString() API returns a flat list of keys which
	// encode the nesting levels of the keys thanks to a dot notation.
	// We reverse engineer that into a map, simplifying the operation
	// thanks to knowing we are dealing with a single level of nesting
	// here: that's dot[0] for a Bindings entry then dot[1] for the key
	// names within that Plan Bindings entry.
	var bindings []map[string]string
	var names []string
	var current_map map[string]string
	var current_name string

	for _, key := range v.AllKeys() {
		dots := strings.Split(key, ".") // we expect a single level
		value := v.GetString(key)

		if current_name == "" || current_name != dots[0] {
			if current_name != "" {
				bindings = append(bindings, current_map)
			}
			current_name = dots[0]
			names = append(names, current_name)
			current_map = make(map[string]string)
		}
		current_map[dots[1]] = value
	}
	// don't forget to finish the current map when out of the loop
	bindings = append(bindings, current_map)

	return &Plan{q, pfile, names, bindings, []ResultSet{}}, nil
}

// Executes a plan and returns the filepath where the output has been
// written, for later comparing
func (p *Plan) Execute(db *sql.DB) error {
	if len(p.Query.Args) == 0 {
		// this Query has no plans, so don't loop over the bindings
		args := make([]interface{}, 0)
		res, err := QueryDB(db, p.Query.OrdinalQuery, args...)

		if err != nil {
			e := fmt.Errorf("Error executing query: %s\n%s\n",
				err,
				p.Query.OrdinalQuery)
			return e
		}
		result := make([]ResultSet, 1)
		result[0] = *res
		p.ResultSets = result
		return nil
	}

	// general case, with a plan and a set of Bindings to go through
	result := make([]ResultSet, len(p.Bindings))

	for i, bindings := range p.Bindings {
		sql, args := p.Query.Prepare(bindings)
		res, err := QueryDB(db, sql, args...)

		if err != nil {
			e := fmt.Errorf(
				"Error executing query with params: %v\n%s\n%s",
				args,
				err,
				sql)
			return e
		}
		result[i] = *res
	}
	p.ResultSets = result
	return nil
}

// WriteResultSets serialize the result of running a query, as a Pretty
// Printed output (comparable to a simplified `psql` output)
func (p *Plan) WriteResultSets(dir string) error {
	for i, rs := range p.ResultSets {
		rsFileName := getResultSetPath(p, dir, i)
		err := rs.Write(rsFileName, true)

		if err != nil {
			e := fmt.Errorf(
				"Failed to write result set '%s': %s\n",
				rsFileName,
				err)
			return e
		}
		p.ResultSets[i].Filename = rsFileName
	}
	return nil
}

// Write a plan to disk in YAML format, thanks to Viper.
func (p *Plan) Write() {
	if len(p.Bindings) == 0 {
		fmt.Printf("Skipping Plan '%s': query uses no variable\n", p.Path)
		return
	}

	fmt.Printf("Creating Empty Plan '%s'\n", p.Path)
	v := viper.New()
	v.SetConfigType("yaml")

	for i, bindings := range p.Bindings {
		for key, value := range bindings {
			vpath := fmt.Sprintf("%s.%s", p.Names[i], key)
			v.Set(vpath, value)
		}
	}
	v.WriteConfigAs(p.Path)
}

func getPlanPath(q *Query, targetdir string) string {
	planPath := filepath.Join(targetdir, filepath.Base(q.Path))
	planPath = strings.TrimSuffix(planPath, path.Ext(planPath))
	planPath = planPath + "_" + q.Name
	planPath = planPath + ".yaml"

	return planPath
}

func getResultSetPath(p *Plan, targetdir string, index int) string {
	var rsFileName string
	basename := strings.TrimSuffix(filepath.Base(p.Path), path.Ext(p.Path))

	if len(p.Query.Args) == 0 {
		rsFileName = fmt.Sprintf("%s.json", basename)
	} else {
		rsFileName = fmt.Sprintf("%s.%s.json", basename, p.Names[index])
	}
	return filepath.Join(targetdir, rsFileName)
}
