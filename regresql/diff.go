package regresql

import (
	"fmt"
	"github.com/pmezard/go-difflib/difflib"
	"os"
)

//readLines reads filename contents and returns a list of strings
func readLines(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)

	if err != nil {
		var data []string
		return data, fmt.Errorf("Failed to read lines from '%s': %s\n",
			filename, err)
	}

	lines := difflib.SplitLines(string(data))
	return lines, nil
}

// DiffFiles returns a unified diff result with c lines of context
func DiffFiles(a string, b string, c int) (string, error) {
	var a_lines, b_lines []string
	var r string
	var err error

	if a_lines, err = readLines(a); err != nil {
		return r, err
	}

	if b_lines, err = readLines(b); err != nil {
		return r, err
	}

	return DiffLines(a, b, a_lines, b_lines, c), nil
}

// DiffLines compares two lists of strings and reports differences in the
// unified format. from and to are the files to report in the diff output.
func DiffLines(from string, to string, a []string, b []string, c int) string {
	diff := difflib.UnifiedDiff{
		A:        a,
		B:        b,
		FromFile: from,
		ToFile:   to,
		Context:  c,
	}
	text, _ := difflib.GetUnifiedDiffString(diff)
	return text
}
