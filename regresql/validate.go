package regresql

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type (
	ValidationResult struct {
		ConfigFile     string
		ConfigValid    bool
		ConfigError    string
		PlanIssues     []ValidationIssue
		SnapshotIssues []ValidationIssue
		Passed         bool
	}

	ValidationIssue struct {
		File    string
		Field   string
		Message string
	}
)

func ValidateForUpgrade(root string) ValidationResult {
	result := ValidationResult{Passed: true}

	configFile := filepath.Join(root, "regresql", "regress.yaml")
	result.ConfigFile = configFile

	cfg, err := ReadConfig(root)
	if err != nil {
		result.ConfigValid = false
		result.ConfigError = err.Error()
		result.Passed = false
		return result
	}
	result.ConfigValid = true

	result.PlanIssues = scanPlanFilesForDeprecated(root)
	if len(result.PlanIssues) > 0 {
		result.Passed = false
	}

	if cfg.Snapshot != nil {
		result.SnapshotIssues = validateSnapshotPaths(root, cfg.Snapshot)
		if len(result.SnapshotIssues) > 0 {
			result.Passed = false
		}
	}

	return result
}

func scanPlanFilesForDeprecated(root string) []ValidationIssue {
	var issues []ValidationIssue

	planDir := filepath.Join(root, "regresql", "plans")
	if _, err := os.Stat(planDir); os.IsNotExist(err) {
		return issues
	}

	planFiles, err := filepath.Glob(filepath.Join(planDir, "*.yaml"))
	if err != nil {
		return issues
	}

	for _, pfile := range planFiles {
		data, err := os.ReadFile(pfile)
		if err != nil {
			continue
		}

		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			continue
		}

		relPath, _ := filepath.Rel(root, pfile)
		if relPath == "" {
			relPath = pfile
		}

		if _, hasFixtures := raw["fixtures"]; hasFixtures {
			issues = append(issues, ValidationIssue{
				File:    relPath,
				Field:   "fixtures",
				Message: "deprecated per-test fixtures",
			})
		}

		if _, hasCleanup := raw["cleanup"]; hasCleanup {
			issues = append(issues, ValidationIssue{
				File:    relPath,
				Field:   "cleanup",
				Message: "deprecated cleanup strategy",
			})
		}
	}

	return issues
}


func validateSnapshotPaths(root string, snap *SnapshotConfig) []ValidationIssue {
	var issues []ValidationIssue

	if snap.Path != "" {
		snapPath := snap.Path
		if !filepath.IsAbs(snapPath) {
			snapPath = filepath.Join(root, snapPath)
		}
		if _, err := os.Stat(snapPath); os.IsNotExist(err) {
			issues = append(issues, ValidationIssue{
				File:    snap.Path,
				Field:   "snapshot.path",
				Message: "snapshot file does not exist",
			})
		}
	}

	if snap.Schema != "" {
		schemaPath := snap.Schema
		if !filepath.IsAbs(schemaPath) {
			schemaPath = filepath.Join(root, schemaPath)
		}
		if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
			issues = append(issues, ValidationIssue{
				File:    snap.Schema,
				Field:   "snapshot.schema",
				Message: "schema file does not exist",
			})
		}
	}

	if snap.Migrations != "" {
		migrationsPath := snap.Migrations
		if !filepath.IsAbs(migrationsPath) {
			migrationsPath = filepath.Join(root, migrationsPath)
		}
		if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
			issues = append(issues, ValidationIssue{
				File:    snap.Migrations,
				Field:   "snapshot.migrations",
				Message: "migrations directory does not exist",
			})
		}
	}

	return issues
}
