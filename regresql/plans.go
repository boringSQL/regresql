package regresql

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/viper"
)

type (
	Plan struct {
		Query        *Query
		Path         string
		Names        []string
		Bindings     []map[string]any
		ResultSets   []ResultSet
		Fixtures     []string          `yaml:"fixtures,omitempty" json:"fixtures,omitempty"`
		Cleanup      CleanupStrategy   `yaml:"cleanup,omitempty" json:"cleanup,omitempty"`
		PlanQuality  *PlanQualityConfig `yaml:"plan_quality,omitempty" json:"plan_quality,omitempty"`
	}

	PlanQualityConfig struct {
		WarnOnSeqScan bool `yaml:"warn_on_seqscan" json:"warn_on_seqscan"`
	}

	TestCase struct {
		Name   string
		Params map[string]any
	}
)

func NewPlan(query *Query, testCases []TestCase) *Plan {
	names := make([]string, len(testCases))
	bindings := make([]map[string]any, len(testCases))

	for i, tc := range testCases {
		names[i] = tc.Name
		bindings[i] = tc.Params
	}

	return &Plan{
		Query:    query,
		Names:    names,
		Bindings: bindings,
	}
}

// CreateEmptyPlan creates a YAML file where to store the set of parameters
// associated with a query.
func (q *Query) CreateEmptyPlan(dir string) (*Plan, error) {
	var names []string
	var bindings []map[string]any
	pfile := getPlanPath(q, dir)

	if _, err := os.Stat(pfile); !os.IsNotExist(err) {
		var p Plan
		return &p, fmt.Errorf("Plan file '%s' already exists", pfile)
	}

	if len(q.NamedArgs) > 0 {
		names = make([]string, 1)
		bindings = make([]map[string]any, 1)

		names[0] = "1"
		bindings[0] = make(map[string]any)
		for _, namedArg := range q.NamedArgs {
			bindings[0][namedArg.Name] = ""
		}
	} else {
		names = []string{}
		bindings = []map[string]any{}
	}

	plan := &Plan{
		Query:      q,
		Path:       pfile,
		Names:      names,
		Bindings:   bindings,
		ResultSets: []ResultSet{},
		Fixtures:   []string{},
		Cleanup:    "",
	}
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
			return &Plan{
				Query:      q,
				Path:       pfile,
				Names:      []string{},
				Bindings:   []map[string]any{},
				ResultSets: []ResultSet{},
				Fixtures:   []string{},
				Cleanup:    "",
			}, nil
		}
		e := fmt.Errorf("Failed to get plan '%s': %s\n", pfile, err)
		return plan, e
	}

	v := viper.New()
	v.SetConfigType("yaml")

	data, err := os.ReadFile(pfile)
	if err != nil {
		return plan, fmt.Errorf("failed to read file '%s': %w", pfile, err)
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
	var bindings []map[string]any
	var names []string
	var current_map map[string]any
	var current_name string

	var fixtures []string
	var cleanup CleanupStrategy

	for _, key := range v.AllKeys() {
		dots := strings.Split(key, ".")

		if len(dots) == 1 {
			if key == "cleanup" {
				cleanup = CleanupStrategy(v.GetString(key))
			}
			continue
		}

		if dots[0] == "fixtures" {
			fixtures = append(fixtures, v.GetString(key))
			continue
		}

		value := v.Get(key)
		if current_name == "" || current_name != dots[0] {
			if current_name != "" {
				bindings = append(bindings, current_map)
			}
			current_name = dots[0]
			names = append(names, current_name)
			current_map = make(map[string]any)
		}
		current_map[dots[1]] = value
	}

	if current_map != nil {
		bindings = append(bindings, current_map)
	}

	planWithFixtures := &Plan{
		Query:      q,
		Path:       pfile,
		Names:      names,
		Bindings:   bindings,
		ResultSets: []ResultSet{},
		Fixtures:   fixtures,
		Cleanup:    cleanup,
	}

	return planWithFixtures, nil
}

// Executes a plan and returns the filepath where the output has been
// written, for later comparing
func (p *Plan) Execute(db *sql.DB) error {
	if len(p.Query.Args) == 0 {
		res, err := QueryDB(db, p.Query.OrdinalQuery)
		if err != nil {
			return fmt.Errorf("error executing query: %w\n%s", err, p.Query.OrdinalQuery)
		}
		p.ResultSets = []ResultSet{*res}
		return nil
	}

	p.ResultSets = make([]ResultSet, len(p.Bindings))

	for i, bindings := range p.Bindings {
		sql, args := p.Query.Prepare(bindings)
		res, err := QueryDB(db, sql, args...)
		if err != nil {
			return fmt.Errorf("error executing query with params %v: %w\n%s", args, err, sql)
		}
		p.ResultSets[i] = *res
	}
	return nil
}

// WriteResultSets serialize the result of running a query, as a Pretty
// Printed output (comparable to a simplified `psql` output)
func (p *Plan) WriteResultSets(dir string) error {
	for i, rs := range p.ResultSets {
		rsFileName := getResultSetPath(p, dir, i)
		if err := rs.Write(rsFileName, true); err != nil {
			return fmt.Errorf("failed to write result set '%s': %w", rsFileName, err)
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
