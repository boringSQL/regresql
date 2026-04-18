package regresql

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed packs/*.yaml
var bundledPacks embed.FS

func resolveAndLoadPack(extends, baseDir string) (config, error) {
	data, source, err := readPack(extends, baseDir)
	if err != nil {
		return config{}, err
	}
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return config{}, fmt.Errorf("parse pack %s: %w", source, err)
	}
	// Chained extends in packs is not supported in v1.
	cfg.Extends = ""
	return cfg, nil
}

func readPack(extends, baseDir string) ([]byte, string, error) {
	if strings.ContainsAny(extends, "/\\") || filepath.IsAbs(extends) {
		path := extends
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, path, err
		}
		return data, path, nil
	}

	name := strings.TrimSuffix(extends, ".yaml") + ".yaml"

	if home, err := os.UserHomeDir(); err == nil {
		shadow := filepath.Join(home, ".regresql", "packs", name)
		if data, err := os.ReadFile(shadow); err == nil {
			return data, shadow, nil
		}
	}

	data, err := bundledPacks.ReadFile("packs/" + name)
	if err != nil {
		return nil, "", fmt.Errorf("unknown pack %q; available: %s",
			extends, strings.Join(listBundledPacks(), ", "))
	}
	return data, "packs/" + name, nil
}

func listBundledPacks() []string {
	entries, err := bundledPacks.ReadDir("packs")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names
}
