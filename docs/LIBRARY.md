# RegreSQL Library Usage

RegreSQL can be used both as a CLI tool and as a Go library for programmatic SQL query execution and baseline capture.

> **Note:** the previous library-level result/cost comparison helpers (`Plan.CompareResultsData`, `Plan.CompareCostsData`, and their `ComparisonResult`/`CostResult` types) have been removed. Use the CLI (`regresql test`, `regresql baseline`) for full regression checking, or call the underlying comparison helpers (`regresql.CompareResultSets`, `regresql.CompareCost`, etc.) directly.

## Quick Start

```go
import "github.com/boringsql/regresql/v2/regresql"

// 1. Create query from string
query, _ := regresql.NewQueryFromString("my-query", "SELECT 1 as num")

// 2. Create test plan
plan := regresql.NewPlan(query, []regresql.TestCase{
    {Name: "test-1"},
})

// 3. Execute
plan.Execute(db)

// 4. Inspect results
for _, rs := range plan.ResultSets {
    fmt.Println(rs.Cols, rs.Rows)
}
```

## Core API

### Query Creation

```go
func NewQueryFromString(name, sqlText string) (*Query, error)
```

Create a query from SQL text (supports `:param` parameters).

```go
query, _ := regresql.NewQueryFromString("get-user", "SELECT * FROM users WHERE id = :user_id")
```

### Plan Creation

```go
type TestCase struct {
    Name   string
    Params map[string]any
}

func NewPlan(query *Query, testCases []TestCase) *Plan
```

Create a test plan with multiple test cases.

```go
plan := regresql.NewPlan(query, []regresql.TestCase{
    {Name: "admin", Params: map[string]any{"user_id": 1}},
    {Name: "user",  Params: map[string]any{"user_id": 42}},
})
```

### Execution

```go
func (p *Plan) Execute(ctx context.Context, q regresql.Querier) error
```

Execute all test cases. `Querier` is satisfied by `*sql.DB`, `*sql.Tx`, etc.

### Baseline Creation

```go
func (p *Plan) CreateBaselines(ctx context.Context, db *sql.DB, useAnalyze bool) ([]regresql.Baseline, []*regresql.ExplainOutput, error)
```

Capture EXPLAIN baselines for each test case. Set `useAnalyze` to `true` to include `BUFFERS` data.

```go
baselines, fullPlans, _ := plan.CreateBaselines(ctx, db, false)
_ = baselines
_ = fullPlans
```

## Comparison Helpers

The library does not wrap high-level regression reporting anymore, but the same primitives the CLI uses are public:

- `regresql.CompareResultSets(expected, actual *ResultSet, config *DiffConfig) *StructuredDiff`
- `regresql.CompareCost(actual, baseline, thresholdPercent float64) (bool, float64)`
- `regresql.CompareBuffers(actual, baseline int64, threshold float64) (bool, float64)`
- `regresql.DetectPlanRegressions(baseline, current *PlanSignature) []PlanRegression`

Use these directly if you need programmatic diffing.

## Storage Patterns

### JSON Storage

```go
import "encoding/json"

// Save results and baselines
labData := struct {
    Expected  []regresql.ResultSet
    Baselines []regresql.Baseline
}{
    Expected:  plan.ResultSets,
    Baselines: baselines,
}
labJSON, _ := json.Marshal(labData)
os.WriteFile("lab-001.json", labJSON, 0644)

// Load lab
data, _ := os.ReadFile("lab-001.json")
var lab struct {
    Expected  []regresql.ResultSet
    Baselines []regresql.Baseline
}
json.Unmarshal(data, &lab)
```

## Comparison: CLI vs Library

| Feature | CLI | Library |
|---------|-----|---------|
| Query Source | .sql files | String input |
| Plan Definition | YAML files | Programmatic |
| Execution | `regresql test` | `plan.Execute(db)` |
| Baselines | `regresql baseline` | `plan.CreateBaselines(db, ...)` |
| Full regression report | Built-in | Use public comparison helpers |
| Use Case | CI/CD, file-based | APIs, web apps, custom tooling |
