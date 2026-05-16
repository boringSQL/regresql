# RegreSQL Library Usage

RegreSQL can be used both as a CLI tool and as a Go library for programmatic SQL query validation.

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

// 4. Compare results
for _, r := range plan.CompareResultsData(expected) {
    if !r.Passed {
        fmt.Println(r.Diff)
    }
}
```

## Core API

### Query Creation

```go
func NewQueryFromString(name, sqlText string) (*Query, error)
```

Create query from SQL text (supports `:param` parameters).

```go
query, _ := regresql.NewQueryFromString("get-user", "SELECT * FROM users WHERE id = :user_id")
```

### Plan Creation

```go
type TestCase struct {
    Name   string
    Params map[string]string
}

func NewPlan(query *Query, testCases []TestCase) *Plan
```

Create test plan with multiple test cases.

```go
plan := regresql.NewPlan(query, []regresql.TestCase{
    {Name: "admin", Params: map[string]string{"user_id": "1"}},
    {Name: "user", Params: map[string]string{"user_id": "42"}},
})
```

### Execution

```go
func (p *Plan) Execute(db *sql.DB) error
```

Execute all test cases.

### Result Validation

```go
type ComparisonResult struct {
    TestName string
    Passed   bool
    Expected string
    Actual   string
    Diff     string
}

func (p *Plan) CompareResultsData(expected []ResultSet) []ComparisonResult
```

Compare actual results against expected.

```go
expected := []regresql.ResultSet{{
    Cols: []string{"id", "name"},
    Rows: [][]any{{1, "Alice"}},
}}

for _, r := range plan.CompareResultsData(expected) {
    if !r.Passed {
        fmt.Printf("Test %s failed:\n%s\n", r.TestName, r.Diff)
    }
}
```

### Performance Validation

```go
type CostResult struct {
    TestName        string
    Passed          bool
    ActualCost      float64
    BaselineCost    float64
    PercentIncrease float64
    Error           string
}

func (p *Plan) CompareCostsData(db *sql.DB, baselines []Baseline, thresholdPercent float64) []CostResult
```

Compare query costs against baselines.

```go
for _, c := range plan.CompareCostsData(db, baselines, 10.0) {
    if !c.Passed {
        fmt.Printf("Cost increased: +%.1f%%\n", c.PercentIncrease)
    }
}
```

### Baseline Creation

```go
func (p *Plan) CreateBaselines(db *sql.DB) ([]Baseline, error)
```

Create performance baselines from query execution.

```go
baselines, _ := plan.CreateBaselines(db)
// Store baselines for later comparison
```

## SQL Labs Examples

### Simple Lab (Single Test Case)

```go
// Setup lab
refQuery, _ := regresql.NewQueryFromString("ref", "SELECT COUNT(*) FROM users")
refPlan := regresql.NewPlan(refQuery, []regresql.TestCase{{Name: "default"}})
refPlan.Execute(db)

expected := refPlan.ResultSets
baselines, _ := refPlan.CreateBaselines(db)

// Validate user submission
userQuery, _ := regresql.NewQueryFromString("user", userSQL)
userPlan := regresql.NewPlan(userQuery, []regresql.TestCase{{Name: "default"}})
userPlan.Execute(db)

// Check correctness
for _, r := range userPlan.CompareResultsData(expected) {
    if !r.Passed {
        fmt.Println("❌", r.Diff)
        return
    }
}

// Check performance
for _, c := range userPlan.CompareCostsData(db, baselines, 20.0) {
    if !c.Passed {
        fmt.Printf("⚠️  Slow: +%.1f%%\n", c.PercentIncrease)
        return
    }
}

fmt.Println("✅ Perfect!")
```

### Advanced Lab (Multiple Test Cases)

```go
refSQL := "SELECT * FROM users WHERE status = :status ORDER BY id"
refQuery, _ := regresql.NewQueryFromString("ref", refSQL)

testCases := []regresql.TestCase{
    {Name: "active", Params: map[string]string{"status": "active"}},
    {Name: "inactive", Params: map[string]string{"status": "inactive"}},
}

refPlan := regresql.NewPlan(refQuery, testCases)
refPlan.Execute(db)

expected := refPlan.ResultSets
baselines, _ := refPlan.CreateBaselines(db)

// Validate user
userQuery, _ := regresql.NewQueryFromString("user", userSQL)
userPlan := regresql.NewPlan(userQuery, testCases)
userPlan.Execute(db)

allPassed := true
for _, r := range userPlan.CompareResultsData(expected) {
    if !r.Passed {
        fmt.Printf("❌ Test '%s' failed\n", r.TestName)
        allPassed = false
    }
}

if allPassed {
    fmt.Println("✅ All tests passed!")
}
```

### API Integration

```go
type Lab struct {
    ID        string
    TestCases []regresql.TestCase
    Expected  []regresql.ResultSet
    Baselines []regresql.Baseline
    Threshold float64
}

type APIResponse struct {
    Success bool   `json:"success"`
    Message string `json:"message"`
    Error   string `json:"error,omitempty"`
    Diff    string `json:"diff,omitempty"`
}

func HandleSubmission(db *sql.DB, lab Lab, userSQL string) APIResponse {
    query, err := regresql.NewQueryFromString("user", userSQL)
    if err != nil {
        return APIResponse{Error: fmt.Sprintf("Syntax: %v", err)}
    }

    plan := regresql.NewPlan(query, lab.TestCases)
    if err := plan.Execute(db); err != nil {
        return APIResponse{Error: fmt.Sprintf("Exec: %v", err)}
    }

    for _, r := range plan.CompareResultsData(lab.Expected) {
        if !r.Passed {
            return APIResponse{
                Success: false,
                Message: "Incorrect results",
                Diff:    r.Diff,
            }
        }
    }

    for _, c := range plan.CompareCostsData(db, lab.Baselines, lab.Threshold) {
        if !c.Passed {
            return APIResponse{
                Success: true,
                Message: fmt.Sprintf("Correct but slow: +%.1f%%", c.PercentIncrease),
            }
        }
    }

    return APIResponse{Success: true, Message: "Perfect!"}
}
```

## Storage Patterns

### JSON Storage

```go
import "encoding/json"

// Save lab
labData := Lab{
    Expected:  plan.ResultSets,
    Baselines: baselines,
}
labJSON, _ := json.Marshal(labData)
os.WriteFile("lab-001.json", labJSON, 0644)

// Load lab
data, _ := os.ReadFile("lab-001.json")
var lab Lab
json.Unmarshal(data, &lab)
```

### Database Storage

```go
// Save as JSONB in PostgreSQL
expectedJSON, _ := json.Marshal(expected)
baselinesJSON, _ := json.Marshal(baselines)

db.Exec(`
    INSERT INTO labs (id, expected, baselines)
    VALUES ($1, $2, $3)
`, labID, expectedJSON, baselinesJSON)

// Load from database
var expectedJSON, baselinesJSON []byte
db.QueryRow("SELECT expected, baselines FROM labs WHERE id = $1", labID).
    Scan(&expectedJSON, &baselinesJSON)

var expected []regresql.ResultSet
var baselines []regresql.Baseline
json.Unmarshal(expectedJSON, &expected)
json.Unmarshal(baselinesJSON, &baselines)
```

## Complete Examples

See `examples/` directory:
- `sqllabs_simple.go` - Basic single-test-case lab
- `sqllabs_advanced.go` - Multiple test cases with parameters
- `sqllabs_api.go` - Web service / REST API integration

Run examples:
```bash
cd examples
go run sqllabs_simple.go
go run sqllabs_advanced.go
go run sqllabs_api.go
```

## Comparison: CLI vs Library

| Feature | CLI | Library |
|---------|-----|---------|
| Query Source | .sql files | String input |
| Plan Definition | YAML files | Programmatic |
| Execution | `regresql test` | `plan.Execute(db)` |
| Results | TAP output | Structured data |
| Storage | JSON files | In-memory |
| Use Case | CI/CD, file-based | APIs, web apps |
