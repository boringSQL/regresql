package regresql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultCostThresholdPercent is the default maximum allowed percentage increase
// for query costs compared to baseline (10% = queries can cost up to 110% of baseline)
const DefaultCostThresholdPercent = 10.0

type (
	Baseline struct {
		Query         string          `json:"query"`
		Timestamp     string          `json:"timestamp"`
		Plan          map[string]any  `json:"plan"`
		PlanSignature *PlanSignature  `json:"plan_signature,omitempty"`
		AnalyzeMode   bool            `json:"analyze_mode,omitempty"`
		Buffers       *BufferBaseline `json:"buffers,omitempty"`
		Actuals       *ActualBaseline `json:"actuals,omitempty"`
	}

	BufferBaseline struct {
		SharedHitBlocks   int64 `json:"shared_hit_blocks"`
		SharedReadBlocks  int64 `json:"shared_read_blocks"`
		TotalBuffers      int64 `json:"total_buffers"`
		TempReadBlocks    int64 `json:"temp_read_blocks"`
		TempWrittenBlocks int64 `json:"temp_written_blocks"`
		TempBuffers       int64 `json:"temp_buffers"`
	}

	ActualBaseline struct {
		ActualRows           float64 `json:"actual_rows"`
		PlanRows             float64 `json:"plan_rows"`
		ExecutionTimeMs      float64 `json:"execution_time_ms"`
		TotalTuplesProcessed float64 `json:"total_tuples_processed,omitempty"`
	}
)

// GetBaselinePath returns the path where baseline JSON file should be stored
func getBaselinePath(q *Query, baselineDir string, bindingName string) string {
	basename := strings.TrimSuffix(filepath.Base(q.Path), filepath.Ext(q.Path))

	// If query name matches file basename, don't duplicate it
	var baselinePath string
	if q.Name == basename {
		baselinePath = filepath.Join(baselineDir, basename)
	} else {
		baselinePath = filepath.Join(baselineDir, basename+"_"+q.Name)
	}

	if bindingName != "" {
		baselinePath = baselinePath + "." + bindingName
	}

	baselinePath = baselinePath + ".json"
	return baselinePath
}

// ExplainOptions configures EXPLAIN execution
type ExplainOptions struct {
	Analyze  bool // Execute query and show actual timing/rows (default: false)
	Buffers  bool // Show buffer usage statistics (default: false)
	Verbose  bool // Show additional output (default: true)
	Settings bool // Show modified configuration parameters (default: true)
}

// DefaultExplainOptions returns safe defaults (no ANALYZE, no BUFFERS)
func DefaultExplainOptions() ExplainOptions {
	return ExplainOptions{
		Analyze:  false,
		Buffers:  false,
		Verbose:  true,
		Settings: true,
	}
}

// ExecuteExplain runs EXPLAIN (FORMAT JSON) with default options (ANALYZE=false)
func ExecuteExplain(ctx context.Context, q Querier, query string, args ...any) (*ExplainOutput, error) {
	return ExecuteExplainWithOptions(ctx, q, query, DefaultExplainOptions(), args...)
}

// ExecuteExplainWithOptions runs EXPLAIN (FORMAT JSON) with configurable options
func ExecuteExplainWithOptions(ctx context.Context, q Querier, query string, opts ExplainOptions, args ...any) (*ExplainOutput, error) {
	explainQuery := fmt.Sprintf(
		"EXPLAIN (FORMAT JSON, ANALYZE %t, VERBOSE %t, COSTS true, BUFFERS %t, SETTINGS %t) %s",
		opts.Analyze, opts.Verbose, opts.Buffers, opts.Settings, query,
	)

	rows, err := q.QueryContext(ctx, explainQuery, args...)
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

	var plans []ExplainOutput
	if err := json.Unmarshal([]byte(jsonPlan), &plans); err != nil {
		return nil, fmt.Errorf("failed to parse EXPLAIN JSON: %w", err)
	}

	if len(plans) == 0 {
		return nil, fmt.Errorf("empty EXPLAIN result")
	}

	return &plans[0], nil
}

func (q *Query) CreateBaseline(ctx context.Context, baselineDir string, planDir string, db *sql.DB, useAnalyze bool) error {
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

	baselines, fullPlans, err := plan.CreateBaselines(ctx, db, useAnalyze)
	if err != nil {
		return err
	}

	for i, baseline := range baselines {
		baselinePath := getBaselinePath(q, baselineDir, plan.Names[i])
		var fullPlan *ExplainOutput
		if i < len(fullPlans) {
			fullPlan = fullPlans[i]
		}
		if err := writeBaselineFile(baseline.Query, baselinePath, baseline.Plan, fullPlan, useAnalyze); err != nil {
			return err
		}
	}

	return nil
}

func writeBaselineFile(queryName, baselinePath string, filteredPlan map[string]any, fullExplainPlan *ExplainOutput, useAnalyze bool) error {
	var planSignature *PlanSignature
	if fullExplainPlan != nil {
		planSignature = ExtractPlanSignatureFromNode(&fullExplainPlan.Plan)
	}

	baseline := Baseline{
		Query:         queryName,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Plan:          filteredPlan,
		PlanSignature: planSignature,
	}

	if useAnalyze && fullExplainPlan != nil {
		baseline.AnalyzeMode = true
		baseline.Buffers = &BufferBaseline{
			SharedHitBlocks:   fullExplainPlan.Plan.SharedHitBlocks,
			SharedReadBlocks:  fullExplainPlan.Plan.SharedReadBlocks,
			TotalBuffers:      fullExplainPlan.Plan.SharedHitBlocks + fullExplainPlan.Plan.SharedReadBlocks,
			TempReadBlocks:    fullExplainPlan.Plan.TempReadBlocks,
			TempWrittenBlocks: fullExplainPlan.Plan.TempWrittenBlocks,
			TempBuffers:       fullExplainPlan.Plan.TempReadBlocks + fullExplainPlan.Plan.TempWrittenBlocks,
		}
		baseline.Actuals = &ActualBaseline{
			ActualRows:           fullExplainPlan.Plan.ActualRows,
			PlanRows:             fullExplainPlan.Plan.PlanRows,
			ExecutionTimeMs:      fullExplainPlan.ExecutionTime,
			TotalTuplesProcessed: SumTuplesProcessed(&fullExplainPlan.Plan),
		}
	}

	jsonBytes, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline to JSON: %w", err)
	}

	if err := os.WriteFile(baselinePath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("failed to write baseline JSON: %w", err)
	}

	mode := ""
	if useAnalyze {
		mode = " [analyze]"
	}
	fmt.Printf("  Created baseline: %s%s\n", filepath.Base(baselinePath), mode)
	return nil
}

type BaselineOptions struct {
	Root      string
	RunFilter string
	Analyze   bool
	Paths     []string
}

func BaselineQueries(opts BaselineOptions) {
	config, err := ReadConfig(opts.Root)
	if err != nil {
		fmt.Printf("Error reading config: %s\n", err.Error())
		os.Exit(3)
	}
	SetGlobalConfig(config)
	useAnalyze := opts.Analyze || IsAnalyzeEnabled()

	if err := TestConnectionString(config.PgUri); err != nil {
		fmt.Printf("Error connecting to database: %s\n", err.Error())
		os.Exit(2)
	}

	db, err := sql.Open("pgx", config.PgUri)
	if err != nil {
		fmt.Printf("Failed to open database connection: %s\n", err.Error())
		os.Exit(2)
	}
	defer db.Close()

	baselineDir := filepath.Join(opts.Root, "regresql", "baselines")

	fmt.Printf("Creating baselines directory: %s\n", baselineDir)
	if err := ensureDir(baselineDir); err != nil {
		fmt.Printf("Failed to create baselines directory: %s\n", err.Error())
		os.Exit(11)
	}

	mode := "cost-based"
	if useAnalyze {
		mode = "analyze (buffers)"
	}
	fmt.Printf("\nCreating baselines for queries (%s):\n", mode)

	plannedQueries, err := WalkPlans(opts.Root)
	if err != nil {
		fmt.Printf("Error walking plans: %s\n", err.Error())
		os.Exit(11)
	}

	// Build a Suite for path filtering
	ignorePatterns := []string{}
	cfg, cfgErr := ReadConfig(opts.Root)
	if cfgErr == nil {
		ignorePatterns = cfg.Ignore
	}
	suite := Walk(opts.Root, ignorePatterns)
	suite.SetRunFilter(opts.RunFilter)
	suite.SetPathFilters(opts.Paths)

	baselineDirs := make(map[string]*lazyDir)

	for _, pq := range plannedQueries {
		fileName := filepath.Base(pq.SQLPath)
		if !suite.matchesRunFilter(fileName, pq.Query.Name) {
			continue
		}
		if !suite.matchesPathFilter(pq.RelPath) {
			continue
		}

		opts := pq.Query.GetRegressQLOptions()
		if opts.NoTest {
			fmt.Printf("  Skipping query '%s' (notest)\n", pq.Query.Name)
			continue
		}
		if opts.NoBaseline {
			fmt.Printf("  Skipping query '%s' (nobaseline)\n", pq.Query.Name)
			continue
		}

		folderDir := filepath.Dir(pq.RelPath)
		dir, ok := baselineDirs[folderDir]
		if !ok {
			dir = &lazyDir{
				path:   filepath.Join(baselineDir, folderDir),
				header: fmt.Sprintf("\n  %s/", folderDir),
			}
			baselineDirs[folderDir] = dir
		}

		if err := dir.Ensure(); err != nil {
			fmt.Printf("Failed to create folder baseline directory: %s\n", err.Error())
			os.Exit(11)
		}

		if err := createBaselineFromPlan(context.Background(), pq, dir.path, db, useAnalyze); err != nil {
			fmt.Printf("  Error creating baseline for %s: %s\n", pq.Query.Name, err.Error())
		}
	}

	fmt.Println("\nBaselines have been created successfully!")
	fmt.Printf("Baseline files are stored in: %s\n", baselineDir)
}

func createBaselineFromPlan(ctx context.Context, pq *PlannedQuery, baselineDir string, db *sql.DB, useAnalyze bool) error {
	q := pq.Query
	plan := pq.Plan

	if len(plan.Bindings) == 0 && len(q.Args) > 0 {
		fmt.Printf("  Skipping '%s': no bindings in plan\n", q.Name)
		return nil
	}

	if len(q.Args) == 0 {
		plan = NewPlan(q, []TestCase{{Name: ""}})
	}

	baselines, fullPlans, err := plan.CreateBaselines(ctx, db, useAnalyze)
	if err != nil {
		return err
	}

	for i, baseline := range baselines {
		baselinePath := getBaselinePath(q, baselineDir, plan.Names[i])
		var fullPlan *ExplainOutput
		if i < len(fullPlans) {
			fullPlan = fullPlans[i]
		}
		if err := writeBaselineFile(baseline.Query, baselinePath, baseline.Plan, fullPlan, useAnalyze); err != nil {
			return err
		}
	}

	return nil
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

func CompareBuffers(actualBuffers, baselineBuffers int64, thresholdPercent float64) (bool, float64) {
	if baselineBuffers == 0 {
		return actualBuffers == 0, 0
	}

	percentageIncrease := (float64(actualBuffers-baselineBuffers) / float64(baselineBuffers)) * 100
	isOk := percentageIncrease <= thresholdPercent

	return isOk, percentageIncrease
}

// IsSpillRegression: temp blocks appeared or grew past threshold.
func IsSpillRegression(actualTemp, baselineTemp int64, thresholdPercent float64) bool {
	ok, _ := CompareBuffers(actualTemp, baselineTemp, thresholdPercent)
	return !ok
}

// CompareTuples checks the tuple no grows compared to baseline
func CompareTuples(actualTuples, baselineTuples, thresholdPercent float64) (bool, float64) {
	if baselineTuples == 0 {
		return true, 0
	}

	percentageIncrease := ((actualTuples - baselineTuples) / baselineTuples) * 100
	isOk := percentageIncrease <= thresholdPercent

	return isOk, percentageIncrease
}
