package cli

import (
	"fmt"
	"os"
)

// Check that given path string is a directory that exists
func checkDirectory(cwd string) error {
	stat, err := os.Stat(cwd)
	if err != nil {
		return fmt.Errorf("No directory found at '%s'\n", cwd)
	}
	if !stat.IsDir() {
		return fmt.Errorf("Not a directory: '%s'\n", cwd)
	}
	return nil
}
