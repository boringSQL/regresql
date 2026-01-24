package regresql

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type (
	PlannedQuery struct {
		Plan     *Plan
		Query    *Query
		SQLPath  string
		PlanPath string
		RelPath  string
	}
)

func WalkPlans(root string) ([]*PlannedQuery, error) {
	planDir := filepath.Join(root, "regresql", "plans")

	if _, err := os.Stat(planDir); os.IsNotExist(err) {
		return nil, nil
	}

	var results []*PlannedQuery
	err := filepath.Walk(planDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		pq, err := loadPlannedQuery(root, path)
		if err != nil {
			return fmt.Errorf("failed to load plan %s: %w", path, err)
		}
		if pq != nil {
			results = append(results, pq)
		}
		return nil
	})

	return results, err
}

func loadPlannedQuery(root, planPath string) (*PlannedQuery, error) {
	planDir := filepath.Join(root, "regresql", "plans")

	relPlanPath, err := filepath.Rel(planDir, planPath)
	if err != nil {
		return nil, err
	}

	folder := filepath.Dir(relPlanPath)
	planFilename := filepath.Base(relPlanPath)
	planBase := strings.TrimSuffix(planFilename, ".yaml")

	sqlDir := filepath.Join(root, folder)
	sqlFile, queryName, err := findSQLFileAndQuery(sqlDir, planBase)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve plan %s: %w", relPlanPath, err)
	}

	sqlPath := filepath.Join(sqlDir, sqlFile)
	relSQLPath := filepath.Join(folder, sqlFile)

	queries, err := parseQueryFile(sqlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SQL file %s: %w", sqlPath, err)
	}

	query, ok := queries[queryName]
	if !ok {
		return nil, fmt.Errorf("query '%s' not found in %s", queryName, sqlPath)
	}

	plan, err := loadPlanFile(planPath, query)
	if err != nil {
		return nil, err
	}

	return &PlannedQuery{
		Plan:     plan,
		Query:    query,
		SQLPath:  sqlPath,
		PlanPath: planPath,
		RelPath:  relSQLPath,
	}, nil
}

// Reverse-engineer SQL file and query name from plan filename
// Plan format: <sqlbase>_<queryname>.yaml -> ("sqlbase.sql", "queryname")
func findSQLFileAndQuery(sqlDir, planBase string) (string, string, error) {
	entries, err := os.ReadDir(sqlDir)
	if err != nil {
		return "", "", fmt.Errorf("cannot read SQL directory %s: %w", sqlDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		sqlBase := strings.TrimSuffix(entry.Name(), ".sql")
		prefix := sqlBase + "_"

		if strings.HasPrefix(planBase, prefix) {
			queryName := strings.TrimPrefix(planBase, prefix)
			if queryName != "" {
				return entry.Name(), queryName, nil
			}
		}
	}

	return "", "", fmt.Errorf("no SQL file found matching plan base '%s' in %s", planBase, sqlDir)
}

func loadPlanFile(planPath string, query *Query) (*Plan, error) {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read plan file '%s': %w", planPath, err)
	}

	return parseYAMLPlan(data, planPath, query)
}
