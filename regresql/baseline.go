package regresql

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// DefaultCostThresholdPercent is the default maximum allowed percentage increase
// for query costs compared to baseline (10% = queries can cost up to 110% of baseline)
const DefaultCostThresholdPercent = 10.0

// Baseline stores the EXPLAIN analysis results for a query
type Baseline struct {
	Query     string         `json:"query"`
	Timestamp string         `json:"timestamp"`
	Plan      map[string]any `json:"plan"`
}

// GetBaselinePath returns the path where baseline JSON file should be stored
func getBaselinePath(q *Query, baselineDir string, bindingName string) string {
	baselinePath := filepath.Join(baselineDir, filepath.Base(q.Path))
	baselinePath = strings.TrimSuffix(baselinePath, filepath.Ext(baselinePath))
	baselinePath = baselinePath + "_" + q.Name

	// If there's a binding name, add it to the filename
	if bindingName != "" {
		baselinePath = baselinePath + "." + bindingName
	}

	baselinePath = baselinePath + ".json"
	return baselinePath
}

// ExecuteExplain runs EXPLAIN (FORMAT JSON) for a query and returns the parsed plan
func ExecuteExplain(db *sql.DB, query string, args ...any) (map[string]any, error) {
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

	var plan []map[string]any
	if err := json.Unmarshal([]byte(jsonPlan), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse EXPLAIN JSON: %w", err)
	}

	if len(plan) == 0 {
		return nil, fmt.Errorf("empty EXPLAIN result")
	}

	return plan[0], nil
}

func (q *Query) CreateBaseline(baselineDir string, planDir string, db *sql.DB, fixtureManager *FixtureManager) error {
	var plan *Plan
	var err error

	if len(q.Args) == 0 {
		plan = NewPlan(q, []TestCase{{Name: ""}})
	} else {
		plan, err = q.GetPlan(planDir)
		if err != nil {
			return fmt.Errorf("failed to load plan for query %s: %w (run 'regresql plan' first)", q.Name, err)
		}
		if len(plan.Bindings) == 0 {
			fmt.Printf("  Skipping '%s': no bindings in plan\n", q.Name)
			return nil
		}
	}

	// Apply fixtures if configured
	if fixtureManager != nil && len(plan.Fixtures) > 0 {
		if err := fixtureManager.BeginTransaction(); err != nil {
			return fmt.Errorf("failed to begin transaction for fixtures: %w", err)
		}
		if err := fixtureManager.ApplyFixtures(plan.Fixtures); err != nil {
			fixtureManager.Rollback()
			return fmt.Errorf("failed to apply fixtures: %w", err)
		}
	}

	baselines, err := plan.CreateBaselines(db)
	if err != nil {
		if fixtureManager != nil && len(plan.Fixtures) > 0 {
			fixtureManager.Rollback()
		}
		return err
	}

	for i, baseline := range baselines {
		baselinePath := getBaselinePath(q, baselineDir, plan.Names[i])
		if err := writeBaselineFile(baseline.Query, baselinePath, baseline.Plan); err != nil {
			if fixtureManager != nil && len(plan.Fixtures) > 0 {
				fixtureManager.Rollback()
			}
			return err
		}
	}

	// Cleanup fixtures
	if fixtureManager != nil && len(plan.Fixtures) > 0 {
		fixture, _ := fixtureManager.LoadFixture(plan.Fixtures[0])
		if plan.Cleanup != "" {
			fixture.Cleanup = plan.Cleanup
		}
		if err := fixtureManager.Cleanup(fixture); err != nil {
			return fmt.Errorf("failed to cleanup fixtures: %w", err)
		}
	}

	return nil
}

func writeBaselineFile(queryName, baselinePath string, filteredPlan map[string]any) error {
	baseline := Baseline{
		Query:     queryName,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Plan:      filteredPlan,
	}

	jsonBytes, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline to JSON: %w", err)
	}

	if err := os.WriteFile(baselinePath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("failed to write baseline JSON: %w", err)
	}

	fmt.Printf("  Created baseline: %s\n", filepath.Base(baselinePath))
	return nil
}

// BaselineQueries creates baselines for all queries in the suite
func BaselineQueries(root string, runFilter string) {
	suite := Walk(root)
	suite.SetRunFilter(runFilter)
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

	var fixtureManager *FixtureManager
	if config.UseFixtures {
		fixtureManager, err = NewFixtureManager(root, db)
		if err != nil {
			fmt.Printf("Failed to create fixture manager: %s\n", err.Error())
			os.Exit(10)
		}
		if err := fixtureManager.IntrospectSchema(); err != nil {
			fmt.Printf("Failed to introspect schema: %s\n", err.Error())
			os.Exit(10)
		}
	}

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
				// Skip if the query doesn't match the run filter
				if !suite.matchesRunFilter(name, q.Name) {
					continue
				}

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

				if err := q.CreateBaseline(folderBaselineDir, folderPlanDir, db, fixtureManager); err != nil {
					fmt.Printf("  Error creating baseline for %s: %s\n", q.Name, err.Error())
				}
			}
		}
	}

	fmt.Println("\nBaselines have been created successfully!")
	fmt.Printf("Baseline files are stored in: %s\n", baselineDir)
}

// LoadBaseline loads a baseline JSON file
func LoadBaseline(baselinePath string) (*Baseline, error) {
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline file '%s': %w", baselinePath, err)
	}

	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("failed to parse baseline JSON '%s': %w", baselinePath, err)
	}

	return &baseline, nil
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
