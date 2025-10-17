package regresql

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/theherk/viper"
)

// DefaultCostThresholdPercent is the default maximum allowed percentage increase
// for query costs compared to baseline (10% = queries can cost up to 110% of baseline)
const DefaultCostThresholdPercent = 10.0

// Baseline stores the EXPLAIN analysis results for a query
type Baseline struct {
	Query     string                 `yaml:"query"`
	Timestamp string                 `yaml:"timestamp"`
	Plan      map[string]interface{} `yaml:"plan"`
}

// GetBaselinePath returns the path where baseline YAML file should be stored
func getBaselinePath(q *Query, baselineDir string, bindingName string) string {
	baselinePath := filepath.Join(baselineDir, filepath.Base(q.Path))
	baselinePath = strings.TrimSuffix(baselinePath, filepath.Ext(baselinePath))
	baselinePath = baselinePath + "_" + q.Name

	// If there's a binding name, add it to the filename
	if bindingName != "" {
		baselinePath = baselinePath + "." + bindingName
	}

	baselinePath = baselinePath + ".yaml"
	return baselinePath
}

// ExecuteExplain runs EXPLAIN (FORMAT JSON) for a query and returns the parsed plan
func ExecuteExplain(db *sql.DB, query string, args ...interface{}) (map[string]interface{}, error) {
	explainQuery := fmt.Sprintf("EXPLAIN (FORMAT JSON, ANALYZE false, VERBOSE false, COSTS true, BUFFERS false) %s", query)

	rows, err := db.Query(explainQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute EXPLAIN: %w", err)
	}
	defer rows.Close()

	var jsonPlan string
	if rows.Next() {
		if err := rows.Scan(&jsonPlan); err != nil {
			return nil, fmt.Errorf("failed to scan EXPLAIN result: %w", err)
		}
	}

	var plan []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPlan), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse EXPLAIN JSON: %w", err)
	}

	if len(plan) == 0 {
		return nil, fmt.Errorf("empty EXPLAIN result")
	}

	return plan[0], nil
}

// CreateBaseline creates a baseline YAML file for a query
func (q *Query) CreateBaseline(baselineDir string, planDir string, db *sql.DB) error {
	// For queries without parameters, create a single baseline
	if len(q.Args) == 0 {
		return q.createSingleBaseline(baselineDir, "", db)
	}

	// For queries with parameters, load the plan and create a baseline for each binding
	plan, err := q.GetPlan(planDir)
	if err != nil {
		return fmt.Errorf("failed to load plan for query %s: %w (run 'regresql plan' first)", q.Name, err)
	}

	// If no bindings in the plan, skip
	if len(plan.Bindings) == 0 {
		fmt.Printf("  Skipping '%s': no bindings in plan\n", q.Name)
		return nil
	}

	// Create a baseline for each binding
	for i, bindings := range plan.Bindings {
		bindingName := plan.Names[i]
		if err := q.createBaselineWithBindings(baselineDir, bindingName, bindings, db); err != nil {
			return err
		}
	}

	return nil
}

// createSingleBaseline creates a baseline for a query without parameters
func (q *Query) createSingleBaseline(baselineDir string, bindingName string, db *sql.DB) error {
	baselinePath := getBaselinePath(q, baselineDir, bindingName)

	explainPlan, err := ExecuteExplain(db, q.OrdinalQuery)
	if err != nil {
		return fmt.Errorf("failed to create baseline for query %s: %w", q.Name, err)
	}

	return writeBaselineFile(q.Name, baselinePath, explainPlan)
}

// createBaselineWithBindings creates a baseline for a query with specific parameter bindings
func (q *Query) createBaselineWithBindings(baselineDir string, bindingName string, bindings map[string]string, db *sql.DB) error {
	baselinePath := getBaselinePath(q, baselineDir, bindingName)

	// Prepare the query with actual parameter values
	sql, args := q.Prepare(bindings)

	explainPlan, err := ExecuteExplain(db, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to create baseline for query %s with bindings %s: %w", q.Name, bindingName, err)
	}

	return writeBaselineFile(q.Name, baselinePath, explainPlan)
}

// writeBaselineFile writes the baseline YAML file
func writeBaselineFile(queryName string, baselinePath string, explainPlan map[string]interface{}) error {
	// Extract the inner Plan object and filter to only keep desired fields
	filteredPlan := make(map[string]interface{})
	if planData, ok := explainPlan["Plan"].(map[string]interface{}); ok {
		if startupCost, ok := planData["Startup Cost"]; ok {
			filteredPlan["startup_cost"] = startupCost
		}
		if totalCost, ok := planData["Total Cost"]; ok {
			filteredPlan["total_cost"] = totalCost
		}
		if planRows, ok := planData["Plan Rows"]; ok {
			filteredPlan["plan_rows"] = planRows
		}
	}

	// Create baseline struct
	baseline := Baseline{
		Query:     queryName,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Plan:      filteredPlan,
	}

	// Write to YAML using viper
	v := viper.New()
	v.SetConfigType("yaml")

	v.Set("query", baseline.Query)
	v.Set("timestamp", baseline.Timestamp)

	for key, value := range baseline.Plan {
		v.Set(fmt.Sprintf("plan.%s", key), value)
	}

	if err := v.WriteConfigAs(baselinePath); err != nil {
		return fmt.Errorf("failed to write baseline YAML: %w", err)
	}

	fmt.Printf("  Created baseline: %s\n", filepath.Base(baselinePath))
	return nil
}

// BaselineQueries creates baselines for all queries in the suite
func BaselineQueries(root string) {
	suite := Walk(root)
	config, err := suite.readConfig()
	if err != nil {
		fmt.Printf("Error reading config: %s\n", err.Error())
		os.Exit(3)
	}

	if err := TestConnectionString(config.PgUri); err != nil {
		fmt.Printf("Error connecting to database: %s\n", err.Error())
		os.Exit(2)
	}

	db, err := sql.Open("postgres", config.PgUri)
	if err != nil {
		fmt.Printf("Failed to open database connection: %s\n", err.Error())
		os.Exit(2)
	}
	defer db.Close()

	baselineDir := filepath.Join(suite.RegressDir, "baselines")

	fmt.Printf("Creating baselines directory: %s\n", baselineDir)
	if err := maybeMkdirAll(baselineDir); err != nil {
		fmt.Printf("Failed to create baselines directory: %s\n", err.Error())
		os.Exit(11)
	}

	fmt.Println("\nCreating baselines for queries:")

	for _, folder := range suite.Dirs {
		folderBaselineDir := filepath.Join(baselineDir, folder.Dir)
		folderPlanDir := filepath.Join(suite.PlanDir, folder.Dir)

		if err := maybeMkdirAll(folderBaselineDir); err != nil {
			fmt.Printf("Failed to create folder baseline directory: %s\n", err.Error())
			os.Exit(11)
		}

		fmt.Printf("\n  %s/\n", folder.Dir)

		for _, name := range folder.Files {
			qfile := filepath.Join(suite.Root, folder.Dir, name)

			queries, err := parseQueryFile(qfile)
			if err != nil {
				fmt.Printf("Error parsing query file %s: %s\n", qfile, err.Error())
				continue
			}

			for _, q := range queries {
				// Skip queries with notest or nobaseline options
				opts := q.GetRegressQLOptions()
				if opts.NoTest {
					fmt.Printf("  Skipping query '%s' (notest)\n", q.Name)
					continue
				}
				if opts.NoBaseline {
					fmt.Printf("  Skipping query '%s' (nobaseline)\n", q.Name)
					continue
				}

				if err := q.CreateBaseline(folderBaselineDir, folderPlanDir, db); err != nil {
					fmt.Printf("  Error creating baseline for %s: %s\n", q.Name, err.Error())
				}
			}
		}
	}

	fmt.Println("\nBaselines have been created successfully!")
	fmt.Printf("Baseline files are stored in: %s\n", baselineDir)
}

// LoadBaseline loads a baseline YAML file
func LoadBaseline(baselinePath string) (*Baseline, error) {
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("baseline file not found: %s", baselinePath)
	}

	v := viper.New()
	v.SetConfigType("yaml")

	data, err := ioutil.ReadFile(baselinePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline file '%s': %w", baselinePath, err)
	}

	if err := v.ReadConfig(bytes.NewBuffer(data)); err != nil {
		return nil, fmt.Errorf("failed to parse baseline YAML '%s': %w", baselinePath, err)
	}

	baseline := &Baseline{
		Query:     v.GetString("query"),
		Timestamp: v.GetString("timestamp"),
		Plan:      make(map[string]interface{}),
	}

	// Load plan fields
	if v.IsSet("plan.startup_cost") {
		baseline.Plan["startup_cost"] = v.Get("plan.startup_cost")
	}
	if v.IsSet("plan.total_cost") {
		baseline.Plan["total_cost"] = v.Get("plan.total_cost")
	}
	if v.IsSet("plan.plan_rows") {
		baseline.Plan["plan_rows"] = v.Get("plan.plan_rows")
	}

	return baseline, nil
}

// CompareCost compares actual cost against baseline with a threshold percentage
// Returns (isOk bool, actual float64, baseline float64, percentage float64)
func CompareCost(actualCost, baselineCost, thresholdPercent float64) (bool, float64) {
	if baselineCost == 0 {
		return actualCost == 0, 0
	}

	percentageIncrease := ((actualCost - baselineCost) / baselineCost) * 100
	isOk := percentageIncrease <= thresholdPercent

	return isOk, percentageIncrease
}
