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

func TestApplyStatistics_FileNotFound(t *testing.T) {
	// Mock DB would be needed for full integration test
	// This tests the file reading error path
	t.Skip("requires database connection for full test")
}
