package regresql

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestWriteEnvHeader(t *testing.T) {
	tests := []struct {
		name string
		env  EnvStatus
		want string
	}{
		{"buffers missing unknown", EnvStatus{Mode: ModeBuffers, PgRegresql: envMissing, GUCs: envGUCsNot, PGMajor: envUnknown},
			"mode: buffers | pg_regresql: missing | GUCs: not | PG major: unknown\n"},
		{"buffers loaded match", EnvStatus{Mode: ModeBuffers, PgRegresql: envLoaded, GUCs: envGUCsNot, PGMajor: envMatch},
			"mode: buffers | pg_regresql: loaded | GUCs: not | PG major: match\n"},
		{"plan choice loaded match", EnvStatus{Mode: ModePlanChoice, PgRegresql: envLoaded, GUCs: envGUCsNot, PGMajor: envMatch},
			"mode: plan choice (injected stats) | pg_regresql: loaded | GUCs: not | PG major: match\n"},
		{"plan choice missing mismatch", EnvStatus{Mode: ModePlanChoice, PgRegresql: envMissing, GUCs: envGUCsNot, PGMajor: envMismatch},
			"mode: plan choice (injected stats) | pg_regresql: missing | GUCs: not | PG major: mismatch\n"},
		{"plan choice loaded unknown", EnvStatus{Mode: ModePlanChoice, PgRegresql: envLoaded, GUCs: envGUCsNot, PGMajor: envUnknown},
			"mode: plan choice (injected stats) | pg_regresql: loaded | GUCs: not | PG major: unknown\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeEnvHeader(&buf, tt.env)
			if buf.String() != tt.want {
				t.Errorf("got %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestEnvFieldsEnumVsLabel(t *testing.T) {
	f := EnvStatus{Mode: ModePlanChoice, PgRegresql: envLoaded, GUCs: envGUCsNot, PGMajor: envMatch}.Fields()
	if f["mode"] != "plan_choice" || f["mode_label"] != "plan choice (injected stats)" {
		t.Errorf("unexpected mode fields: %v", f)
	}
	if f["pg_regresql"] != "loaded" || f["gucs"] != "not" || f["pg_major"] != "match" {
		t.Errorf("unexpected fields: %v", f)
	}
	b := EnvStatus{Mode: ModeBuffers}.Fields()
	if b["mode"] != "buffers" || b["mode_label"] != "buffers" {
		t.Errorf("unexpected buffers fields: %v", b)
	}
}

func TestConsoleStartPrintsHeaderBeforeRunning(t *testing.T) {
	f := &ConsoleFormatter{}
	f.SetEnv(EnvStatus{Mode: ModeBuffers, PgRegresql: envMissing, GUCs: envGUCsNot, PGMajor: envUnknown})
	var buf bytes.Buffer
	if err := f.Start(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	hi := strings.Index(out, "mode: buffers")
	ri := strings.Index(out, "Running regression tests...")
	if hi < 0 || ri < 0 || hi > ri {
		t.Errorf("header must precede Running line: %q", out)
	}
}

func TestConsoleStartWithoutEnvUnchanged(t *testing.T) {
	var buf bytes.Buffer
	if err := (&ConsoleFormatter{}).Start(&buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "\nRunning regression tests...\n" {
		t.Errorf("got %q", buf.String())
	}
}

func TestJSONFormatterEnvironment(t *testing.T) {
	var buf bytes.Buffer
	f := &JSONFormatter{}
	s := NewTestSummary()
	s.Env = EnvStatus{Mode: ModePlanChoice, PgRegresql: envLoaded, GUCs: envGUCsNot, PGMajor: envMatch}
	if err := f.Finish(s, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`"environment"`, `"mode": "plan_choice"`, `"mode_label": "plan choice (injected stats)"`, `"pg_regresql": "loaded"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}

func TestProgressWriter(t *testing.T) {
	tests := []struct {
		format, out string
		stderr      bool
	}{
		{"", "", false},
		{"console", "", false},
		{"json", "", true},
		{"junit", "", true},
		{"github-actions", "", true},
		{"pgtap", "", true},
		{"json", "report.json", false},
	}
	for _, tt := range tests {
		got := progressWriter(tt.format, tt.out)
		want := io.Writer(os.Stdout)
		if tt.stderr {
			want = os.Stderr
		}
		if got != want {
			t.Errorf("progressWriter(%q, %q) wrong stream", tt.format, tt.out)
		}
	}
}

func TestConnectionStringReportsToProgress(t *testing.T) {
	var buf bytes.Buffer
	old := progress
	progress = &buf
	t.Cleanup(func() { progress = old })
	_ = TestConnectionString("postgres://u@127.0.0.1:1/x?connect_timeout=1")
	if !strings.Contains(buf.String(), "Connecting to") {
		t.Errorf("got %q", buf.String())
	}
}
