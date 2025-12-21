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
		Path              string    `yaml:"path"`
		Hash              string    `yaml:"hash"`
		Created           time.Time `yaml:"created"`
		SizeBytes         int64     `yaml:"size_bytes"`
		Format            string    `yaml:"format"`
		SchemaPath        string    `yaml:"schema_path,omitempty"`
		SchemaHash        string    `yaml:"schema_hash,omitempty"`
		MigrationsDir     string    `yaml:"migrations_dir,omitempty"`
		MigrationsHash    string    `yaml:"migrations_hash,omitempty"`
		MigrationsApplied []string  `yaml:"migrations_applied,omitempty"`
		FixturesUsed      []string  `yaml:"fixtures_used,omitempty"`
	}

	SnapshotFormat string

	SnapshotOptions struct {
		OutputPath string
		Format     SnapshotFormat
		SchemaOnly bool
		Section    string
	}

	SectionsOptions struct {
		OutputDir string
	}

	SectionInfo struct {
		Section   string
		Path      string
		Hash      string
		SizeBytes int64
	}

	SectionsResult struct {
		Sections []SectionInfo
		Created  time.Time
	}

	RestoreOptions struct {
		InputPath string
		Format    SnapshotFormat
		Clean     bool // drop existing objects before restore
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

// RestoreTool returns the appropriate PostgreSQL tool for restoring this format.
func (f SnapshotFormat) RestoreTool() string {
	if f == FormatPlain {
		return "psql"
	}
	return "pg_restore"
}

// CheckPgTool verifies that a PostgreSQL client tool is available and meets version requirements.
// If .tool-versions specifies postgres version, the tool must be at least that major version.
func CheckPgTool(tool, projectRoot string) error {
	if _, err := exec.LookPath(tool); err != nil {
		return fmt.Errorf("%s is required but not found in PATH. Please install PostgreSQL client tools", tool)
	}

	requiredMajor := parseToolVersions(filepath.Join(projectRoot, ".tool-versions"))
	if requiredMajor == 0 {
		return nil
	}

	installedMajor := parseToolMajorVersion(tool)
	if installedMajor > 0 && installedMajor < requiredMajor {
		return fmt.Errorf("%s version %d is older than postgres %d in .tool-versions", tool, installedMajor, requiredMajor)
	}
	return nil
}

// parseToolVersions extracts postgres major version from .tool-versions file.
func parseToolVersions(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if version, ok := strings.CutPrefix(line, "postgres "); ok {
			var major int
			fmt.Sscanf(version, "%d", &major)
			return major
		}
	}
	return 0
}

// parseToolMajorVersion extracts major version from a pg tool's --version output.
func parseToolMajorVersion(tool string) int {
	out, err := exec.Command(tool, "--version").Output()
	if err != nil {
		return 0
	}
	if fields := strings.Fields(string(out)); len(fields) >= 3 {
		var major int
		fmt.Sscanf(fields[len(fields)-1], "%d", &major)
		return major
	}
	return 0
}

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
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
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

// CaptureSections captures all three database sections (pre-data, data, post-data)
// to separate plain SQL files for git-friendly version control.
func CaptureSections(pguri string, opts SectionsOptions) (*SectionsResult, error) {
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	sections := []string{"pre-data", "data", "post-data"}
	result := &SectionsResult{
		Created: time.Now().UTC(),
	}

	for _, section := range sections {
		outputPath := filepath.Join(opts.OutputDir, section+".sql")

		info, err := captureSection(pguri, section, outputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to capture %s section: %w", section, err)
		}

		result.Sections = append(result.Sections, *info)
	}

	return result, nil
}

func captureSection(pguri, section, outputPath string) (*SectionInfo, error) {
	args := []string{"--dbname", pguri, "--format=plain", "--section", section}

	cmd := exec.Command("pg_dump", args...)
	cmd.Stderr = os.Stderr

	outFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	cmd.Stdout = outFile

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pg_dump failed: %w", err)
	}

	stat, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output file: %w", err)
	}

	hash, err := computeSingleFileHash(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash: %w", err)
	}

	return &SectionInfo{
		Section:   section,
		Path:      outputPath,
		Hash:      hash,
		SizeBytes: stat.Size(),
	}, nil
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

	if err := os.WriteFile(metadataPath, data, 0o644); err != nil {
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

// RestoreSnapshot restores a database snapshot using pg_restore or psql
func RestoreSnapshot(pguri string, opts RestoreOptions) error {
	if _, err := os.Stat(opts.InputPath); os.IsNotExist(err) {
		return fmt.Errorf("snapshot file not found: %s", opts.InputPath)
	}

	format := opts.Format
	if format == "" {
		format = DetectSnapshotFormat(opts.InputPath)
	}

	if format == FormatPlain {
		return restoreWithPsql(pguri, opts)
	}
	return restoreWithPgRestore(pguri, opts, format)
}

func restoreWithPgRestore(pguri string, opts RestoreOptions, format SnapshotFormat) error {
	args := []string{"--dbname", pguri}

	if opts.Clean {
		args = append(args, "--clean", "--if-exists")
	}

	switch format {
	case FormatCustom:
		args = append(args, "--format=custom")
	case FormatDirectory:
		args = append(args, "--format=directory")
	}

	args = append(args, opts.InputPath)

	cmd := exec.Command("pg_restore", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore failed: %w", err)
	}
	return nil
}

func restoreWithPsql(pguri string, opts RestoreOptions) error {
	args := []string{pguri, "-f", opts.InputPath}

	if !opts.Clean {
		args = append(args, "-v", "ON_ERROR_STOP=1")
	}

	cmd := exec.Command("psql", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql failed: %w", err)
	}
	return nil
}

func DetectSnapshotFormat(path string) SnapshotFormat {
	stat, err := os.Stat(path)
	if err != nil {
		return FormatCustom
	}

	if stat.IsDir() {
		return FormatDirectory
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".sql" {
		return FormatPlain
	}

	return FormatCustom
}

func computeSchemaHash(schemaPath string) (string, error) {
	format := DetectSnapshotFormat(schemaPath)
	return computeFileHash(schemaPath, format)
}

func ValidateSchemaHash(root string) error {
	snapshotsDir := GetSnapshotsDir(root)

	metadata, err := ReadSnapshotMetadata(snapshotsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: no snapshot metadata found. Consider using 'regresql snapshot build' for reproducible tests.\n")
		return nil
	}

	if metadata.Current == nil || metadata.Current.SchemaPath == "" {
		return nil
	}

	// Check if schema file still exists
	if _, err := os.Stat(metadata.Current.SchemaPath); os.IsNotExist(err) {
		// Schema file referenced in metadata doesn't exist - stale metadata
		return nil
	}

	currentHash, err := computeSchemaHash(metadata.Current.SchemaPath)
	if err != nil {
		return fmt.Errorf("failed to hash schema %s: %w", metadata.Current.SchemaPath, err)
	}

	if currentHash != metadata.Current.SchemaHash {
		return fmt.Errorf(`schema has changed since last snapshot build

  Schema file: %s
  Expected:    %s
  Current:     %s

Run 'regresql snapshot build --schema=%s' to rebuild the snapshot`,
			metadata.Current.SchemaPath,
			metadata.Current.SchemaHash[:20]+"...",
			currentHash[:20]+"...",
			metadata.Current.SchemaPath)
	}

	return nil
}
