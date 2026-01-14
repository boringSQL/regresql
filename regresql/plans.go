package regresql

import (
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
		Query       *Query
		Path        string
		Names       []string
		Bindings    []map[string]any
		ResultSets  []ResultSet
		PlanQuality *PlanQualityConfig `yaml:"plan_quality,omitempty" json:"plan_quality,omitempty"`
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
	var planQuality *PlanQualityConfig

	// Reject deprecated fixtures and cleanup fields with clear error messages
	if _, hasFixtures := raw["fixtures"]; hasFixtures {
		return nil, fmt.Errorf("'fixtures:' in plan file '%s' is no longer supported.\n"+
			"Per-test fixtures have been removed. Use snapshots instead:\n"+
			"  1. Move fixture data to regresql/fixtures/\n"+
			"  2. Run 'regresql snapshot build'\n"+
			"  3. Remove 'fixtures:' from this plan file", pfile)
	}
	if _, hasCleanup := raw["cleanup"]; hasCleanup {
		return nil, fmt.Errorf("'cleanup:' in plan file '%s' is no longer supported.\n"+
			"Cleanup strategies have been removed. Tests now always run in transactions that rollback.\n"+
			"Remove 'cleanup:' from this plan file.", pfile)
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
		PlanQuality: planQuality,
	}, nil
}

// Execute runs the plan's query against the given querier (db or transaction)
func (p *Plan) Execute(q Querier) error {
	if os.Getenv("REGRESQL_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[DEBUG] Executing query %s with %d bindings: %v\n", p.Query.Name, len(p.Bindings), p.Names)
	}

	if len(p.Query.Args) == 0 {
		res, err := RunQuery(q, p.Query.OrdinalQuery)
		if err != nil {
			return fmt.Errorf("error executing query: %w\n%s", err, p.Query.OrdinalQuery)
		}
		p.ResultSets = []ResultSet{*res}
		return nil
	}

	p.ResultSets = make([]ResultSet, len(p.Bindings))
	for i, bindings := range p.Bindings {
		sql, args := p.Query.Prepare(bindings)
		res, err := RunQuery(q, sql, args...)
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

// ComputeDiffForInteractive computes a diff between current results and existing expected files
// Returns a human-readable diff string for interactive review
func (p *Plan) ComputeDiffForInteractive(expectedDir string) string {
	var diffs []string

	for i, rs := range p.ResultSets {
		expectedPath := getResultSetPath(p, expectedDir, i)

		// Check if expected file exists
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			diffs = append(diffs, fmt.Sprintf("  [NEW] %s (%d rows)", filepath.Base(expectedPath), len(rs.Rows)))
			continue
		}

		// Load existing expected result
		expected, err := LoadResultSet(expectedPath)
		if err != nil {
			diffs = append(diffs, fmt.Sprintf("  [ERROR] %s: %v", filepath.Base(expectedPath), err))
			continue
		}

		// Compare row counts
		if len(expected.Rows) != len(rs.Rows) {
			diffs = append(diffs, fmt.Sprintf("  [CHANGED] %s: %d rows → %d rows",
				filepath.Base(expectedPath), len(expected.Rows), len(rs.Rows)))
			continue
		}

		// Quick check if content differs (compare JSON representations)
		expectedJSON := expected.ToJSON()
		actualJSON := rs.ToJSON()
		if expectedJSON != actualJSON {
			diffs = append(diffs, fmt.Sprintf("  [CHANGED] %s: content differs", filepath.Base(expectedPath)))
		}
	}

	if len(diffs) == 0 {
		return "  (no changes)"
	}
	return strings.Join(diffs, "\n")
}
