package regresql

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadPackConfig(t *testing.T, name string) Config {
	t.Helper()
	data, source, err := readPack(name, "")
	if err != nil {
		t.Fatalf("readPack(%q): %v", name, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse pack %s: %v", source, err)
	}
	cfg.Extends = ""
	return cfg
}

func TestBundledPacksLoad(t *testing.T) {
	packs := listBundledPacks()
	if len(packs) == 0 {
		t.Fatal("expected at least one bundled pack")
	}
	for _, name := range packs {
		cfg := loadPackConfig(t, name)
		if cfg.Policies == nil {
			t.Errorf("pack %q: expected non-nil Policies", name)
		}
	}
}

func TestExtendsName_Embedded(t *testing.T) {
	tmp := t.TempDir()
	regressDir := filepath.Join(tmp, "regresql")
	if err := os.MkdirAll(regressDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(regressDir, "regress.yaml"),
		[]byte("extends: fintech\npguri: postgres://localhost\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := ReadConfig(tmp)
	if err != nil {
		t.Fatalf("ReadConfig error = %v", err)
	}
	if cfg.Extends != "" {
		t.Errorf("Extends should be cleared after resolve, got %q", cfg.Extends)
	}
	if cfg.Policies == nil || cfg.Policies.Severity["seq_scan_critical_table"] != "error" {
		t.Errorf("expected fintech severity for seq_scan_critical_table=error, got %+v", cfg.Policies)
	}
	if cfg.PgUri != "postgres://localhost" {
		t.Errorf("project PgUri should survive merge, got %q", cfg.PgUri)
	}
}

func TestExtendsPath_Relative(t *testing.T) {
	tmp := t.TempDir()
	regressDir := filepath.Join(tmp, "regresql")
	if err := os.MkdirAll(regressDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(regressDir, "my-pack.yaml"),
		[]byte("policies:\n  severity:\n    sort_added: error\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(regressDir, "regress.yaml"),
		[]byte("extends: ./my-pack.yaml\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := ReadConfig(tmp)
	if err != nil {
		t.Fatalf("ReadConfig error = %v", err)
	}
	if cfg.Policies == nil || cfg.Policies.Severity["sort_added"] != "error" {
		t.Errorf("expected local pack to apply sort_added=error, got %+v", cfg.Policies)
	}
}

func TestExtendsName_UserShadow(t *testing.T) {
	fakeHome := t.TempDir()
	shadowDir := filepath.Join(fakeHome, ".regresql", "packs")
	if err := os.MkdirAll(shadowDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(shadowDir, "fintech.yaml"),
		[]byte("policies:\n  severity:\n    seq_scan_critical_table: info\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", fakeHome)

	tmp := t.TempDir()
	regressDir := filepath.Join(tmp, "regresql")
	if err := os.MkdirAll(regressDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(regressDir, "regress.yaml"),
		[]byte("extends: fintech\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := ReadConfig(tmp)
	if err != nil {
		t.Fatalf("ReadConfig error = %v", err)
	}
	if got := cfg.Policies.Severity["seq_scan_critical_table"]; got != "info" {
		t.Errorf("user shadow must win; seq_scan_critical_table=%q, want %q", got, "info")
	}
}

func TestExtendsUnknownPack(t *testing.T) {
	tmp := t.TempDir()
	regressDir := filepath.Join(tmp, "regresql")
	if err := os.MkdirAll(regressDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(regressDir, "regress.yaml"),
		[]byte("extends: does_not_exist\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	_, err := ReadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for unknown pack")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown pack") {
		t.Errorf("error should mention 'unknown pack', got: %v", err)
	}
	if !strings.Contains(msg, "fintech") {
		t.Errorf("error should list available packs (fintech), got: %v", err)
	}
}

func TestExtendsNoChaining(t *testing.T) {
	tmp := t.TempDir()
	regressDir := filepath.Join(tmp, "regresql")
	if err := os.MkdirAll(regressDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(regressDir, "base.yaml"),
		[]byte("extends: fintech\npolicies:\n  severity:\n    sort_added: info\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(regressDir, "regress.yaml"),
		[]byte("extends: ./base.yaml\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := ReadConfig(tmp)
	if err != nil {
		t.Fatalf("ReadConfig error = %v", err)
	}
	// base.yaml's own `extends: fintech` must be ignored, so we should see
	// base.yaml's sort_added=info but NOT the fintech seq_scan_critical_table=error.
	if cfg.Policies == nil {
		t.Fatal("expected Policies from base.yaml")
	}
	if cfg.Policies.Severity["sort_added"] != "info" {
		t.Errorf("expected base pack's sort_added=info, got %q", cfg.Policies.Severity["sort_added"])
	}
	if _, present := cfg.Policies.Severity["seq_scan_critical_table"]; present {
		t.Errorf("chained extends should be ignored; fintech's severity must not leak through")
	}
}

func TestExtendsLayering(t *testing.T) {
	cases := []struct {
		name, pack, project string
		got                 func(Config) any
		want                any
	}{
		{"project scalar overrides pack", "analyze: {buffer_threshold: 5}", "analyze: {buffer_threshold: 3}",
			func(c Config) any { return c.Analyze.BufferThreshold }, 3.0},
		{"project zero overrides pack", "analyze: {buffer_threshold: 5}", "analyze: {buffer_threshold: 0}",
			func(c Config) any { return c.Analyze.BufferThreshold }, 0.0},
		{"pack scalar survives unrelated project key", "analyze: {buffer_threshold: 5}", "analyze: {cost_threshold: 7}",
			func(c Config) any { return [2]float64{c.Analyze.BufferThreshold, c.Analyze.CostThreshold} }, [2]float64{5, 7}},
		{"omitted analyze key keeps default", "", "analyze: {enabled: true}",
			func(c Config) any { return c.Analyze.BufferThreshold }, 2.0},
		{"ignore appends and dedupes", "ignore: [a, b]", "ignore: [b, c]",
			func(c Config) any { return []string(c.Ignore) }, []string{"a", "b", "c"}},
		{"fixtures append", "snapshot: {fixtures: [one.sql]}", "snapshot: {fixtures: [two.sql]}",
			func(c Config) any { return []string(c.Snapshot.Fixtures) }, []string{"one.sql", "two.sql"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			root := t.TempDir()
			dir := filepath.Join(root, "regresql")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{"pack.yaml": tc.pack, "regress.yaml": "extends: ./pack.yaml\n" + tc.project}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body+"\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := ReadConfig(root)
			if err != nil {
				t.Fatalf("ReadConfig: %v", err)
			}
			if got := tc.got(cfg); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadPack_YamlSuffixOptional(t *testing.T) {
	a, _, errA := readPack("fintech", "")
	b, _, errB := readPack("fintech.yaml", "")
	if errA != nil || errB != nil {
		t.Fatalf("readPack errors: %v / %v", errA, errB)
	}
	if string(a) != string(b) {
		t.Error("fintech and fintech.yaml must resolve to the same pack")
	}
}
