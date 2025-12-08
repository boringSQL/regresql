package regresql

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateConfigField(t *testing.T) {
	tmpDir := t.TempDir()
	regressDir := filepath.Join(tmpDir, "regresql")
	if err := os.Mkdir(regressDir, 0755); err != nil {
		t.Fatalf("failed to create regresql dir: %v", err)
	}

	// create initial config
	initialConfig := `root: .
pguri: postgres://old@localhost/testdb
`
	configFile := filepath.Join(regressDir, "regress.yaml")
	if err := os.WriteFile(configFile, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// update pguri
	newUri := "postgres://new@newhost/newdb"
	if err := UpdateConfigField(tmpDir, "pguri", newUri); err != nil {
		t.Fatalf("UpdateConfigField() error = %v", err)
	}

	// read back and verify
	cfg, err := ReadConfig(tmpDir)
	if err != nil {
		t.Fatalf("ReadConfig() error = %v", err)
	}

	if cfg.PgUri != newUri {
		t.Errorf("PgUri = %q, want %q", cfg.PgUri, newUri)
	}

	// root should be preserved
	if cfg.Root != "." {
		t.Errorf("Root = %q, want %q (should be preserved)", cfg.Root, ".")
	}
}

func TestUpdateConfigFieldUnsupportedKey(t *testing.T) {
	tmpDir := t.TempDir()
	regressDir := filepath.Join(tmpDir, "regresql")
	if err := os.Mkdir(regressDir, 0755); err != nil {
		t.Fatalf("failed to create regresql dir: %v", err)
	}

	configFile := filepath.Join(regressDir, "regress.yaml")
	if err := os.WriteFile(configFile, []byte("pguri: test\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	err := UpdateConfigField(tmpDir, "unsupported", "value")
	if err == nil {
		t.Error("UpdateConfigField() should return error for unsupported key")
	}
}
