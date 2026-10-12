package regresql

import "testing"

func TestMinPGVersionConstant(t *testing.T) {
	if MinPGVersionForStats != 180000 {
		t.Errorf("MinPGVersionForStats = %d, want 180000", MinPGVersionForStats)
	}
}

func TestVersionCheckLogic(t *testing.T) {
	tests := []struct {
		name       string
		versionNum int
		wantErr    bool
	}{
		{"PG16 should fail", 160002, true},
		{"PG17 should fail", 170004, true},
		{"PG18 minimum should pass", 180000, false},
		{"PG18.1 should pass", 180001, false},
		{"PG19 should pass", 190000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			belowMinVersion := tt.versionNum < MinPGVersionForStats
			if belowMinVersion != tt.wantErr {
				t.Errorf("versionNum=%d: belowMinVersion=%v, wantErr=%v",
					tt.versionNum, belowMinVersion, tt.wantErr)
			}
		})
	}
}

func TestEstimateMatchesInjected(t *testing.T) {
	tests := []struct {
		name      string
		est       float64
		reltuples float64
		want      bool
	}{
		{"exact", 1000000, 1000000, true},
		{"within tolerance", 700000, 1000000, true},
		{"tracks physical file", 1, 1000000, false},
		{"at the edge", 499999, 1000000, true},
		{"just outside", 499998, 1000000, false},
		{"zero reltuples, clamped to one row", 1, 0, true},
		{"zero reltuples, real estimate", 100, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := estimateMatchesInjected(tt.est, tt.reltuples); got != tt.want {
				t.Errorf("estimateMatchesInjected(%v, %v) = %v, want %v", tt.est, tt.reltuples, got, tt.want)
			}
		})
	}
}

func TestStripPsqlMeta(t *testing.T) {
	in := "--\n-- Dumped from database version 18.0\n--\n\n\\restrict abc123\n\nSELECT * FROM pg_catalog.pg_restore_relation_stats('relname', 't');\n\\unrestrict abc123\n"
	want := "--\n-- Dumped from database version 18.0\n--\n\n\nSELECT * FROM pg_catalog.pg_restore_relation_stats('relname', 't');\n"
	if got := stripPsqlMeta(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := stripPsqlMeta("\\restrict tok\r\nSELECT 1;\r\n\\unrestrict tok"); got != "SELECT 1;\r\n" {
		t.Errorf("CRLF: got %q", got)
	}
}

func TestStripPsqlMetaKeepsLiteralContinuation(t *testing.T) {
	in := "SELECT f('most_common_vals', '{\"a\nb\n\\\"q\\\"\n\\restricted}');\n"
	if got := stripPsqlMeta(in); got != in {
		t.Errorf("literal changed: %q", got)
	}
	multi := "SELECT '{x,\n\\\"q\\\"}';\n"
	if got := stripPsqlMeta(multi); got != multi {
		t.Errorf("literal changed: %q", got)
	}
}
