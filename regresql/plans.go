package regresql

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gopkg.in/yaml.v3"
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

// GetPlan instantiates a Plan from a Query, parsing a set of actual
// parameters when it exists.
func (q *Query) GetPlan(planDir string) (*Plan, error) {
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
		return nil, fmt.Errorf("Failed to get plan '%s': %s\n", pfile, err)
	}

	data, err := os.ReadFile(pfile)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", pfile, err)
	}

	// Unmarshal into generic map to extract all fields
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse YAML file '%s': %w", pfile, err)
	}

	// Extract known top-level fields
	var fixtures []string
	var cleanup CleanupStrategy
	var planQuality *PlanQualityConfig

	if fixturesRaw, ok := raw["fixtures"]; ok {
		if fixturesList, ok := fixturesRaw.([]any); ok {
			for _, f := range fixturesList {
				if str, ok := f.(string); ok {
					fixtures = append(fixtures, str)
				}
			}
		}
		delete(raw, "fixtures")
	}

	if cleanupRaw, ok := raw["cleanup"]; ok {
		if str, ok := cleanupRaw.(string); ok {
			cleanup = CleanupStrategy(str)
		}
		delete(raw, "cleanup")
	}

	if planQualityRaw, ok := raw["plan_quality"]; ok {
		// Re-marshal and unmarshal to convert to struct
		if pqData, err := yaml.Marshal(planQualityRaw); err == nil {
			var pq PlanQualityConfig
			if err := yaml.Unmarshal(pqData, &pq); err == nil {
				planQuality = &pq
			}
		}
		delete(raw, "plan_quality")
	}

	// Remaining keys are bindings - extract and sort them for consistent ordering
	var names []string
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)

	// Build bindings array from sorted names
	bindings := make([]map[string]any, 0, len(names))
	for _, name := range names {
		bindingData := raw[name]
		if bindingMap, ok := bindingData.(map[string]any); ok {
			bindings = append(bindings, bindingMap)
		}
	}

	return &Plan{
		Query:       q,
		Path:        pfile,
		Names:       names,
		Bindings:    bindings,
		ResultSets:  []ResultSet{},
		Fixtures:    fixtures,
		Cleanup:     cleanup,
		PlanQuality: planQuality,
	}, nil
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

	// Debug: Log execution details
	if os.Getenv("REGRESQL_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[DEBUG] Executing query %s with %d bindings: %v\n", p.Query.Name, len(p.Bindings), p.Names)
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

// Write a plan to disk in YAML format.
func (p *Plan) Write() {
	if len(p.Bindings) == 0 {
		fmt.Printf("Skipping Plan '%s': query uses no variable\n", p.Path)
		return
	}

	fmt.Printf("Creating Empty Plan '%s'\n", p.Path)

	// Build the YAML structure
	planData := make(map[string]any)

	// Add bindings
	for i, bindings := range p.Bindings {
		planData[p.Names[i]] = bindings
	}

	// Add optional fields
	if len(p.Fixtures) > 0 {
		planData["fixtures"] = p.Fixtures
	}
	if p.Cleanup != "" {
		planData["cleanup"] = p.Cleanup
	}
	if p.PlanQuality != nil {
		planData["plan_quality"] = p.PlanQuality
	}

	// Marshal to YAML
	data, err := yaml.Marshal(planData)
	if err != nil {
		fmt.Printf("Error marshaling plan to YAML: %s\n", err)
		return
	}

	// Write to file
	if err := os.WriteFile(p.Path, data, 0644); err != nil {
		fmt.Printf("Error writing plan file '%s': %s\n", p.Path, err)
	}
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
