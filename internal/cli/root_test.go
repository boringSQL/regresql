package cli

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestBuildVersion(t *testing.T) {
	info := func(v string) *debug.BuildInfo { return &debug.BuildInfo{Main: debug.Module{Version: v}} }
	tests := []struct {
		name string
		v    string
		info *debug.BuildInfo
		want string
	}{
		{"ldflags wins", "v2.1.0", info("v9.9.9"), "v2.1.0"},
		{"no build info", "dev", nil, "dev"},
		{"devel build", "dev", info("(devel)"), "dev"},
		{"empty module version", "dev", info(""), "dev"},
		{"dirty tree", "dev", info("v2.0.1-0.20261021-abc+dirty"), "dev"},
		{"go install at a tag", "dev", info("v2.1.0"), "v2.1.0"},
	}
	for _, tt := range tests {
		if got := buildVersion(tt.v, tt.info); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestAddWithoutPathNamesTheArgument(t *testing.T) {
	err := addCmd.Args(addCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "regresql add <file|dir|glob>") {
		t.Errorf("unhelpful error: %v", err)
	}
	if err := addCmd.Args(addCmd, []string{"q.sql"}); err != nil {
		t.Errorf("one path must be accepted: %v", err)
	}
}

func TestLeftoverCommandsAreHidden(t *testing.T) {
	if !listCmd.Hidden || !validateConfigCmd.Hidden {
		t.Error("list and validate-config must stay out of --help")
	}
}

func TestRootSilencesErrorsSoMainPrintsOnce(t *testing.T) {
	if !RootCmd.SilenceErrors {
		t.Error("main prints the error; cobra must not print it too")
	}
}
