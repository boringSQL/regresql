package regresql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundledPacksLoad(t *testing.T) {
	packs := listBundledPacks()
	if len(packs) == 0 {
		t.Fatal("expected at least one bundled pack")
	}
	for _, name := range packs {
		cfg, err := resolveAndLoadPack(name, "")
		if err != nil {
			t.Errorf("resolveAndLoadPack(%q) error = %v", name, err)
			continue
		}
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

func TestMergeConfig_ScalarProjectWins(t *testing.T) {
	base := config{PgUri: "base-uri", Root: "base-root"}
	over := config{PgUri: "over-uri"}
	out := mergeConfig(base, over)
	if out.PgUri != "over-uri" {
		t.Errorf("PgUri: expected project win, got %q", out.PgUri)
	}
	if out.Root != "base-root" {
		t.Errorf("Root: expected base preserved, got %q", out.Root)
	}
}

func TestMergeConfig_ListsConcatDedupe(t *testing.T) {
	base := config{Ignore: []string{"a", "b"}}
	over := config{Ignore: []string{"b", "c"}}
	out := mergeConfig(base, over)
	want := []string{"a", "b", "c"}
	if len(out.Ignore) != len(want) {
		t.Fatalf("Ignore length = %d, want %d (%v)", len(out.Ignore), len(want), out.Ignore)
	}
	for i, v := range want {
		if out.Ignore[i] != v {
			t.Errorf("Ignore[%d] = %q, want %q", i, out.Ignore[i], v)
		}
	}
}

func TestMergeConfig_MapKeyMerge(t *testing.T) {
	base := config{Policies: &PoliciesConfig{
		Severity: map[string]string{"a": "warning", "b": "warning"},
	}}
	over := config{Policies: &PoliciesConfig{
		Severity: map[string]string{"b": "error", "c": "info"},
	}}
	out := mergeConfig(base, over)
	if out.Policies.Severity["a"] != "warning" {
		t.Errorf("a: expected base preserved")
	}
	if out.Policies.Severity["b"] != "error" {
		t.Errorf("b: expected project override, got %q", out.Policies.Severity["b"])
	}
	if out.Policies.Severity["c"] != "info" {
		t.Errorf("c: expected project addition")
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
