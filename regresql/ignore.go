package regresql

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type IgnoreMatcher struct {
	patterns []string
	root     string
}

func NewIgnoreMatcher(root string, patterns []string) *IgnoreMatcher {
	return &IgnoreMatcher{
		patterns: patterns,
		root:     root,
	}
}

func LoadIgnoreFile(root string) (*IgnoreMatcher, error) {
	ignoreFile := filepath.Join(root, ".regresignore")

	if _, err := os.Stat(ignoreFile); os.IsNotExist(err) {
		return NewIgnoreMatcher(root, []string{}), nil
	}

	file, err := os.Open(ignoreFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return NewIgnoreMatcher(root, patterns), nil
}

func (im *IgnoreMatcher) ShouldIgnore(path string, isDir bool) bool {
	relPath, err := filepath.Rel(im.root, path)
	if err != nil {
		return false
	}

	if relPath == "regresql" || strings.HasPrefix(relPath, "regresql"+string(filepath.Separator)) {
		return true
	}

	for _, pattern := range im.patterns {
		if matchPattern(pattern, relPath, isDir) {
			return true
		}
	}

	return false
}

func matchPattern(pattern, relPath string, isDir bool) bool {
	dirOnly := strings.HasSuffix(pattern, "/")
	if dirOnly {
		pattern = strings.TrimSuffix(pattern, "/")
		if !isDir {
			return false
		}
	}

	if strings.HasPrefix(pattern, "/") {
		pattern = strings.TrimPrefix(pattern, "/")
		return matchPath(pattern, relPath, isDir)
	}

	if strings.Contains(pattern, "/") {
		return matchPath(pattern, relPath, isDir)
	}

	// Pattern without slash matches basename anywhere
	base := filepath.Base(relPath)
	matched, _ := filepath.Match(pattern, base)
	return matched
}

func matchPath(pattern, relPath string, isDir bool) bool {
	if strings.Contains(pattern, "**") {
		return matchGlobstar(pattern, relPath)
	}

	matched, _ := filepath.Match(pattern, relPath)
	if matched {
		return true
	}

	// For directories, check if pattern is a prefix
	if isDir && strings.HasPrefix(relPath+string(filepath.Separator), pattern+string(filepath.Separator)) {
		return true
	}

	return false
}

func matchGlobstar(pattern, relPath string) bool {
	idx := strings.Index(pattern, "**")
	if idx == -1 {
		return false
	}

	prefix := pattern[:idx]
	suffix := pattern[idx+2:]

	prefix = strings.TrimSuffix(prefix, "/")
	suffix = strings.TrimPrefix(suffix, "/")

	if prefix != "" && !strings.HasPrefix(relPath, prefix) {
		return false
	}

	if suffix != "" {
		if suffix == "*" {
			return true
		}

		// Check if suffix matches end of path
		parts := strings.Split(relPath, string(filepath.Separator))
		for i := 0; i < len(parts); i++ {
			remaining := filepath.Join(parts[i:]...)
			matched, _ := filepath.Match(suffix, remaining)
			if matched {
				return true
			}
		}
		return false
	}

	return true
}
