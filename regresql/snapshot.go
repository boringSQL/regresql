package regresql

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type (
	SnapshotMetadata struct {
		Current *SnapshotInfo `yaml:"current"`
	}

	SnapshotInfo struct {
		Path         string    `yaml:"path"`
		Hash         string    `yaml:"hash"`
		Created      time.Time `yaml:"created"`
		SizeBytes    int64     `yaml:"size_bytes"`
		Format       string    `yaml:"format"`
		FixturesUsed []string  `yaml:"fixtures_used,omitempty"`
	}

	SnapshotFormat string

	SnapshotOptions struct {
		OutputPath string
		Format     SnapshotFormat
		SchemaOnly bool
		Section    string
	}
)

const (
	FormatCustom    SnapshotFormat = "custom"
	FormatPlain     SnapshotFormat = "plain"
	FormatDirectory SnapshotFormat = "directory"

	DefaultSnapshotPath   = "snapshots/default.dump"
	DefaultSnapshotFormat = FormatCustom
	SnapshotMetadataFile  = ".regresql-snapshot.yaml"
)

// CaptureSnapshot captures the current database state using pg_dump
func CaptureSnapshot(pguri string, opts SnapshotOptions) (*SnapshotInfo, error) {
	if opts.Format == "" {
		opts.Format = DefaultSnapshotFormat
	}
	if err := validateFormat(opts.Format); err != nil {
		return nil, err
	}

	if opts.Section != "" {
		if err := validateSection(opts.Section); err != nil {
			return nil, err
		}
	}

	outputDir := filepath.Dir(opts.OutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	args := buildPgDumpArgs(pguri, opts)

	cmd := exec.Command("pg_dump", args...)
	cmd.Stderr = os.Stderr

	// plain format outputs to stdout, others write directly to file
	if opts.Format == FormatPlain {
		outFile, err := os.Create(opts.OutputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create output file: %w", err)
		}
		defer outFile.Close()
		cmd.Stdout = outFile
	}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pg_dump failed: %w", err)
	}

	stat, err := os.Stat(opts.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat snapshot file: %w", err)
	}

	hash, err := computeFileHash(opts.OutputPath, opts.Format)
	if err != nil {
		return nil, fmt.Errorf("failed to compute snapshot hash: %w", err)
	}

	info := &SnapshotInfo{
		Path:      opts.OutputPath,
		Hash:      hash,
		Created:   time.Now().UTC(),
		SizeBytes: stat.Size(),
		Format:    string(opts.Format),
	}

	return info, nil
}

func buildPgDumpArgs(pguri string, opts SnapshotOptions) []string {
	args := []string{"--dbname", pguri}

	switch opts.Format {
	case FormatCustom:
		args = append(args, "--format=custom", "--file", opts.OutputPath)
	case FormatDirectory:
		args = append(args, "--format=directory", "--file", opts.OutputPath)
	case FormatPlain:
		args = append(args, "--format=plain")
	}

	if opts.SchemaOnly {
		args = append(args, "--schema-only")
	}

	if opts.Section != "" {
		args = append(args, "--section", opts.Section)
	}

	return args
}

func validateFormat(format SnapshotFormat) error {
	switch format {
	case FormatCustom, FormatPlain, FormatDirectory:
		return nil
	default:
		return fmt.Errorf("invalid snapshot format: %s (must be custom, plain, or directory)", format)
	}
}

func validateSection(section string) error {
	switch section {
	case "pre-data", "data", "post-data":
		return nil
	default:
		return fmt.Errorf("invalid section: %s (must be pre-data, data, or post-data)", section)
	}
}

func computeFileHash(path string, format SnapshotFormat) (string, error) {
	if format == FormatDirectory {
		return computeDirectoryHash(path)
	}
	return computeSingleFileHash(path)
}

func computeSingleFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func computeDirectoryHash(dirPath string) (string, error) {
	h := sha256.New()

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(dirPath, path)
		h.Write([]byte(relPath))

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func WriteSnapshotMetadata(snapshotsDir string, info *SnapshotInfo) error {
	metadataPath := filepath.Join(snapshotsDir, SnapshotMetadataFile)

	metadata := SnapshotMetadata{Current: info}

	data, err := yaml.Marshal(&metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot metadata: %w", err)
	}

	return nil
}

func ReadSnapshotMetadata(snapshotsDir string) (*SnapshotMetadata, error) {
	metadataPath := filepath.Join(snapshotsDir, SnapshotMetadataFile)

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot metadata: %w", err)
	}

	var metadata SnapshotMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot metadata: %w", err)
	}

	return &metadata, nil
}

func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func GetSnapshotPath(cfg *SnapshotConfig, root string) string {
	if cfg != nil && cfg.Path != "" {
		if !filepath.IsAbs(cfg.Path) {
			return filepath.Join(root, cfg.Path)
		}
		return cfg.Path
	}
	return filepath.Join(root, DefaultSnapshotPath)
}

func GetSnapshotFormat(cfg *SnapshotConfig) SnapshotFormat {
	if cfg != nil && cfg.Format != "" {
		return SnapshotFormat(strings.ToLower(cfg.Format))
	}
	return DefaultSnapshotFormat
}

func GetSnapshotsDir(root string) string {
	return filepath.Join(root, "snapshots")
}
