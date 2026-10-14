package regresql

import (
	"os"
	"path/filepath"
	"testing"
)

func setInjected(t *testing.T, v bool) {
	t.Helper()
	old := injectedStats
	injectedStats = v
	t.Cleanup(func() { injectedStats = old })
}

func TestEnvStatusFailed(t *testing.T) {
	tests := []struct {
		name  string
		env   EnvStatus
		fail  bool
		abort bool
	}{
		{"plan choice missing", EnvStatus{Mode: ModePlanChoice, PgRegresql: envMissing}, true, true},
		{"buffers missing", EnvStatus{Mode: ModeBuffers, PgRegresql: envMissing}, false, false},
		{"plan choice loaded", EnvStatus{Mode: ModePlanChoice, PgRegresql: envLoaded, PGMajor: envMatch}, false, false},
		{"plan choice unknown probe", EnvStatus{Mode: ModePlanChoice, PgRegresql: envUnknown}, false, false},
		{"mismatch plan choice", EnvStatus{Mode: ModePlanChoice, PgRegresql: envLoaded, PGMajor: envMismatch}, true, false},
		{"major unknown", EnvStatus{Mode: ModePlanChoice, PgRegresql: envLoaded, PGMajor: envUnknown}, false, false},
		{"tripwire", EnvStatus{Mode: ModePlanChoice, PgRegresql: envLoaded, Tripwire: true}, true, false},
		{"snapshot major detail only", EnvStatus{Mode: ModePlanChoice, PgRegresql: envLoaded, PGMajor: envMatch, Detail: []string{"snapshot was built on PostgreSQL 16, server is 18"}}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.env.Failed(); got != tt.fail {
				t.Errorf("Failed() = %v, want %v", got, tt.fail)
			}
			if got := tt.env.AbortRun(); got != tt.abort {
				t.Errorf("AbortRun() = %v, want %v", got, tt.abort)
			}
		})
	}
}

func TestStatsFileMajor(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	late := ""
	for i := 0; i < 25; i++ {
		late += "-- filler\n"
	}
	late += "-- Dumped from database version 18.0\n"

	tests := []struct {
		name string
		path string
		want int
	}{
		{"header", write("a.sql", "--\n-- Dumped from database version 18.1\n--\nSELECT 1;\n"), 18},
		{"no header", write("b.sql", "SELECT 1;\n"), 0},
		{"garbage", write("c.sql", "-- Dumped from database version abc\n"), 0},
		{"beyond line 20", write("d.sql", late), 0},
		{"missing file", filepath.Join(dir, "nope.sql"), 0},
		{"empty path", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statsFileMajor(tt.path); got != tt.want {
				t.Errorf("statsFileMajor = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSnapshotMajorGuards(t *testing.T) {
	tests := []struct {
		name string
		meta *SnapshotMetadata
		want int
	}{
		{"nil meta", nil, 0},
		{"nil current", &SnapshotMetadata{}, 0},
		{"nil server", &SnapshotMetadata{Current: &SnapshotInfo{}}, 0},
		{"zero version", &SnapshotMetadata{Current: &SnapshotInfo{Server: &ServerContext{}}}, 0},
		{"set", &SnapshotMetadata{Current: &SnapshotInfo{Server: &ServerContext{VersionNum: 160002}}}, 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := snapshotMajor(tt.meta); got != tt.want {
				t.Errorf("snapshotMajor = %d, want %d", got, tt.want)
			}
		})
	}
}
