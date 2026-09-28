package regresql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckUnfilled(t *testing.T) {
	tests := []struct {
		name      string
		plan      string
		runFilter string
		wantErr   string
	}{
		{name: "filled", plan: "\"1\":\n  id: 42\n"},
		{name: "empty string allowed", plan: "\"1\":\n  id: \"\"\n"},
		{name: "required", plan: "\"1\":\n  id: REQUIRED\n", wantErr: "q.yaml: 1.id"},
		{name: "qshape placeholder", plan: "\"1\":\n  id: REPLACE_ME\n", wantErr: "q.yaml: 1.id"},
		{name: "case without body keeps names aligned", plan: "\"1\":\n\"2\":\n  id: REQUIRED\n", wantErr: "q.yaml: 2.id"},
		{name: "filtered out", plan: "\"1\":\n  id: REQUIRED\n", runFilter: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, filepath.Join(root, "q.sql"), "select * from t where id = :id;\n")
			writeTestFile(t, filepath.Join(root, "other.sql"), "select 1;\n")
			writeTestFile(t, filepath.Join(root, "regresql", "plans", "q.yaml"), tt.plan)
			writeTestFile(t, filepath.Join(root, "regresql", "plans", "other.yaml"), "{}\n")

			pqs, err := WalkPlans(root)
			if err != nil {
				t.Fatal(err)
			}
			suite := Walk(root, nil)
			suite.SetRunFilter(tt.runFilter)

			err = suite.checkUnfilled(pqs)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestAddedPlanIsRefused(t *testing.T) {
	root := t.TempDir()
	sqlPath := filepath.Join(root, "q.sql")
	writeTestFile(t, sqlPath, "select * from t where id = :id and name = :name;\n")
	queries, err := parseQueryFile(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	planDir := filepath.Join(root, "regresql", "plans")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := queries["q"].CreateEmptyPlan(planDir); err != nil {
		t.Fatal(err)
	}

	pqs, err := WalkPlans(root)
	if err != nil {
		t.Fatal(err)
	}
	err = Walk(root, nil).checkUnfilled(pqs)
	if err == nil || !strings.Contains(err.Error(), "q.yaml: 1.id") || !strings.Contains(err.Error(), "q.yaml: 1.name") {
		t.Fatalf("error = %v, want both 1.id and 1.name listed", err)
	}
}
