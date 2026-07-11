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

func writeTestConfig(t *testing.T, pguri string) string {
	t.Helper()
	tmpDir := t.TempDir()
	regressDir := filepath.Join(tmpDir, "regresql")
	if err := os.Mkdir(regressDir, 0755); err != nil {
		t.Fatalf("failed to create regresql dir: %v", err)
	}
	body := "root: .\npguri: " + pguri + "\n"
	if err := os.WriteFile(filepath.Join(regressDir, "regress.yaml"), []byte(body), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return tmpDir
}

func TestReadConfigDatabaseURLOverride(t *testing.T) {
	const fileURI = "postgres://file@localhost/filedb"
	const envURI = "postgres://env@localhost/envdb"
	tmpDir := writeTestConfig(t, fileURI)

	t.Setenv("DATABASE_URL", envURI)
	cfg, err := ReadConfig(tmpDir)
	if err != nil {
		t.Fatalf("ReadConfig() error = %v", err)
	}
	if cfg.PgUri != envURI {
		t.Errorf("PgUri = %q, want env override %q", cfg.PgUri, envURI)
	}

	// ReadConfigFile is raw: env must not leak in
	raw, err := ReadConfigFile(tmpDir)
	if err != nil {
		t.Fatalf("ReadConfigFile() error = %v", err)
	}
	if raw.PgUri != fileURI {
		t.Errorf("ReadConfigFile PgUri = %q, want file value %q", raw.PgUri, fileURI)
	}
}

func TestReadConfigNoOverrideWhenUnset(t *testing.T) {
	const fileURI = "postgres://file@localhost/filedb"
	tmpDir := writeTestConfig(t, fileURI)

	t.Setenv("DATABASE_URL", "")
	cfg, err := ReadConfig(tmpDir)
	if err != nil {
		t.Fatalf("ReadConfig() error = %v", err)
	}
	if cfg.PgUri != fileURI {
		t.Errorf("PgUri = %q, want file value %q", cfg.PgUri, fileURI)
	}
}

// config set must not persist the DATABASE_URL value into regress.yaml
func TestUpdateConfigFieldIgnoresEnvOverride(t *testing.T) {
	const fileURI = "postgres://file@localhost/filedb"
	const envURI = "postgres://env@localhost/envdb"
	const newURI = "postgres://new@localhost/newdb"
	tmpDir := writeTestConfig(t, fileURI)

	t.Setenv("DATABASE_URL", envURI)
	if err := UpdateConfigField(tmpDir, "pguri", newURI); err != nil {
		t.Fatalf("UpdateConfigField() error = %v", err)
	}

	raw, err := ReadConfigFile(tmpDir)
	if err != nil {
		t.Fatalf("ReadConfigFile() error = %v", err)
	}
	if raw.PgUri != newURI {
		t.Errorf("persisted PgUri = %q, want %q (env must not leak in)", raw.PgUri, newURI)
	}
}
