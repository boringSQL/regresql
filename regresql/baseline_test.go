package regresql

import "testing"

func TestBaselineExitCode(t *testing.T) {
	if baselineExitCode(0) != 0 || baselineExitCode(1) != 13 || baselineExitCode(5) != 13 {
		t.Error("unexpected baseline exit mapping")
	}
}
