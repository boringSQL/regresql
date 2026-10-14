package regresql

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func restoreGlobalConfig(t *testing.T) {
	t.Helper()
	old := cachedConfig
	t.Cleanup(func() { cachedConfig = old })
}

func unreachableProject(t *testing.T) string {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
	restoreGlobalConfig(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "regresql"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "pguri: postgres://u@127.0.0.1:1/x?connect_timeout=1\n"
	if err := os.WriteFile(filepath.Join(root, "regresql", "regress.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	setInjected(t, false)
	return root
}

func TestDoctorJSONStdoutIsValidWithMissingStatsFile(t *testing.T) {
	root := unreachableProject(t)

	var out bytes.Buffer
	env, err := Doctor(DoctorOptions{Root: root, Stats: filepath.Join(root, "missing.sql"), Format: "json", Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if got["mode"] != "buffers" || got["ok"] != false {
		t.Errorf("unexpected json: %v", got)
	}
	detail, _ := got["detail"].([]any)
	found := false
	for _, d := range detail {
		if strings.Contains(d.(string), "stats file not found") {
			found = true
		}
	}
	if !found {
		t.Errorf("stats warning must be in detail: %v", detail)
	}
	if env.Connected {
		t.Error("port 1 must not connect")
	}
}

func TestDoctorTextHeader(t *testing.T) {
	root := unreachableProject(t)

	var out bytes.Buffer
	if _, err := Doctor(DoctorOptions{Root: root, Format: "text", Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "mode: buffers | pg_regresql: unknown | GUCs: not | PG major: unknown\n") {
		t.Errorf("unexpected header: %q", out.String())
	}
}

func TestDoctorConfigError(t *testing.T) {
	var out bytes.Buffer
	if _, err := Doctor(DoctorOptions{Root: t.TempDir(), Format: "json", Out: &out}); err == nil {
		t.Error("expected config error")
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got["ok"] != false || got["error"] == nil {
		t.Errorf("expected JSON error object, got %q (%v)", out.String(), err)
	}

	out.Reset()
	if _, err := Doctor(DoctorOptions{Root: t.TempDir(), Format: "text", Out: &out}); err == nil || out.Len() != 0 {
		t.Errorf("text mode must write nothing to stdout, got %q", out.String())
	}
}
