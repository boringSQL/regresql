package regresql

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type (
	SnapshotMetadata struct {
		Current *SnapshotInfo   `yaml:"current"`
		History []*SnapshotInfo `yaml:"history,omitempty"`
	}

	SnapshotInfo struct {
		Path                 string         `yaml:"path"`
		Hash                 string         `yaml:"hash"`
		Created              time.Time      `yaml:"created"`
		SizeBytes            int64          `yaml:"size_bytes"`
		Format               string         `yaml:"format"`
		Tag                  string         `yaml:"tag,omitempty"`
		Note                 string         `yaml:"note,omitempty"`
		SchemaPath           string         `yaml:"schema_path,omitempty"`
		SchemaHash           string         `yaml:"schema_hash,omitempty"`
		MigrationsDir        string         `yaml:"migrations_dir,omitempty"`
		MigrationsHash       string         `yaml:"migrations_hash,omitempty"`
		MigrationsApplied    []string       `yaml:"migrations_applied,omitempty"`
		MigrationCommand     string         `yaml:"migration_command,omitempty"`
		MigrationCommandHash string         `yaml:"migration_command_hash,omitempty"`
		FixturesUsed         []string       `yaml:"fixtures_used,omitempty"`
		Server               *ServerContext `yaml:"server,omitempty"`
	}

	ServerContext struct {
		Version         string            `yaml:"version"`
		VersionNum      int               `yaml:"version_num"`
		PlannerSettings map[string]string `yaml:"planner_settings"`
	}

	SettingsDiff struct {
		Name     string
		Expected string
		Actual   string
	}

	ServerValidation struct {
		VersionMatch  bool
		MajorMismatch bool
		VersionDiff   *SettingsDiff
		SettingsDiffs []SettingsDiff
	}

	ValidateSettingsMode string

	RestoreState struct {
		SnapshotPath   string    `yaml:"snapshot_path"`
		SnapshotMtime  time.Time `yaml:"snapshot_mtime"`
		SnapshotSize   int64     `yaml:"snapshot_size"`
		Database       string    `yaml:"database"`
		RestoredAt     time.Time `yaml:"restored_at"`
		DurationMillis int64     `yaml:"duration_millis"`
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
		InputPath      string
		Format         SnapshotFormat
		Clean          bool   // drop existing objects before restore
		TargetDatabase string // override database name from connection string
	}
)

const (
	FormatCustom    SnapshotFormat = "custom"
	FormatPlain     SnapshotFormat = "plain"
	FormatDirectory SnapshotFormat = "directory"

	DefaultSnapshotPath   = "snapshots/default.dump"
	DefaultSnapshotFormat = FormatCustom
	SnapshotMetadataFile  = ".regresql-snapshot.yaml"
	RestoreStateFile      = ".regresql-restore-state.yaml"

	ValidateSettingsWarn   ValidateSettingsMode = "warn"
	ValidateSettingsStrict ValidateSettingsMode = "strict"
	ValidateSettingsIgnore ValidateSettingsMode = "ignore"
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

	// Load existing metadata to preserve history
	var metadata SnapshotMetadata
	if existing, err := ReadSnapshotMetadata(snapshotsDir); err == nil {
		metadata.History = existing.History
	}
	metadata.Current = info

	data, err := yaml.Marshal(&metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write snapshot metadata: %w", err)
	}

	return nil
}

// WriteSnapshotMetadataFull writes the complete metadata including history
func WriteSnapshotMetadataFull(snapshotsDir string, metadata *SnapshotMetadata) error {
	metadataPath := filepath.Join(snapshotsDir, SnapshotMetadataFile)

	data, err := yaml.Marshal(metadata)
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

func ShouldAutoRestore(cfg *SnapshotConfig) bool {
	if cfg == nil || cfg.Path == "" {
		return false
	}
	if cfg.AutoRestore == nil {
		return true
	}
	return *cfg.AutoRestore
}

func ReadRestoreState(snapshotsDir string) (*RestoreState, error) {
	statePath := filepath.Join(snapshotsDir, RestoreStateFile)
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}
	var state RestoreState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func WriteRestoreState(snapshotsDir string, state *RestoreState) error {
	statePath := filepath.Join(snapshotsDir, RestoreStateFile)
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, data, 0o644)
}

func NeedsRestore(snapshotsDir, snapshotPath, targetDB string) (bool, string) {
	state, err := ReadRestoreState(snapshotsDir)
	if err != nil {
		return true, "no previous restore state"
	}

	if state.SnapshotPath != snapshotPath {
		return true, "snapshot path changed"
	}

	if state.Database != targetDB {
		return true, "target database changed"
	}

	stat, err := os.Stat(snapshotPath)
	if err != nil {
		return true, "failed to stat snapshot"
	}

	if stat.Size() != state.SnapshotSize {
		return true, "snapshot size changed"
	}

	if !stat.ModTime().Equal(state.SnapshotMtime) {
		return true, "snapshot modified"
	}

	return false, ""
}

// replaceDatabase returns a new connection string with a different database
func replaceDatabase(pguri, newDB string) (string, error) {
	u, err := url.Parse(pguri)
	if err != nil {
		return "", err
	}
	u.Path = "/" + newDB
	return u.String(), nil
}

// RestoreSnapshot restores a database snapshot using pg_restore or psql
func RestoreSnapshot(pguri string, opts RestoreOptions) error {
	if _, err := os.Stat(opts.InputPath); os.IsNotExist(err) {
		return fmt.Errorf("snapshot file not found: %s", opts.InputPath)
	}

	// Override target database if specified
	targetURI := pguri
	if opts.TargetDatabase != "" {
		var err error
		targetURI, err = replaceDatabase(pguri, opts.TargetDatabase)
		if err != nil {
			return fmt.Errorf("failed to set target database: %w", err)
		}
	}

	format := opts.Format
	if format == "" {
		format = DetectSnapshotFormat(opts.InputPath)
	}

	if format == FormatPlain {
		return restoreWithPsql(targetURI, opts)
	}
	return restoreWithPgRestore(targetURI, opts, format)
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

func ValidateMigrationsHash(root string) error {
	snapshotsDir := GetSnapshotsDir(root)

	metadata, err := ReadSnapshotMetadata(snapshotsDir)
	if err != nil {
		return nil // No metadata - already warned by ValidateSchemaHash
	}

	info := metadata.Current
	if info == nil || info.MigrationsDir == "" {
		return nil
	}

	if _, err := os.Stat(info.MigrationsDir); os.IsNotExist(err) {
		return nil // Stale metadata - directory no longer exists
	}

	currentFiles, err := discoverMigrations(info.MigrationsDir)
	if err != nil {
		return fmt.Errorf("failed to discover migrations in %s: %w", info.MigrationsDir, err)
	}

	if len(currentFiles) == 0 && info.MigrationsHash == "" {
		return nil // No migrations before, none now
	}

	if len(currentFiles) == 0 {
		return migrationChangeError(info, "", nil, info.MigrationsApplied)
	}

	currentHash, err := computeMigrationsHash(currentFiles)
	if err != nil {
		return fmt.Errorf("failed to hash migrations: %w", err)
	}

	if currentHash == info.MigrationsHash {
		return nil
	}

	// Detect what changed
	currentNames := make([]string, len(currentFiles))
	for i, f := range currentFiles {
		currentNames[i] = filepath.Base(f)
	}

	return migrationChangeError(info, currentHash, currentNames, info.MigrationsApplied)
}

func migrationChangeError(info *SnapshotInfo, currentHash string, current, stored []string) error {
	currentSet := make(map[string]bool)
	for _, name := range current {
		currentSet[name] = true
	}
	storedSet := make(map[string]bool)
	for _, name := range stored {
		storedSet[name] = true
	}

	var added, removed []string
	for _, name := range current {
		if !storedSet[name] {
			added = append(added, name)
		}
	}
	for _, name := range stored {
		if !currentSet[name] {
			removed = append(removed, name)
		}
	}

	var changes strings.Builder
	changes.WriteString("\n  Changes detected:")
	if len(added) == 0 && len(removed) == 0 {
		changes.WriteString("\n    ~ content modified")
	}
	for _, name := range added {
		changes.WriteString("\n    + ")
		changes.WriteString(name)
	}
	for _, name := range removed {
		changes.WriteString("\n    - ")
		changes.WriteString(name)
	}

	expectedHash := info.MigrationsHash
	if expectedHash != "" {
		expectedHash = expectedHash[:20] + "..."
	} else {
		expectedHash = "(none)"
	}
	if currentHash != "" {
		currentHash = currentHash[:20] + "..."
	} else {
		currentHash = "(empty)"
	}

	return fmt.Errorf(`migrations have changed since last snapshot build

  Migrations dir: %s
  Expected hash:  %s
  Current hash:   %s%s

Run 'regresql snapshot build --migrations=%s' to rebuild the snapshot`,
		info.MigrationsDir, expectedHash, currentHash, changes.String(), info.MigrationsDir)
}

func ValidateMigrationCommandHash(root string) error {
	snapshotsDir := GetSnapshotsDir(root)

	metadata, err := ReadSnapshotMetadata(snapshotsDir)
	if err != nil {
		return nil // No metadata
	}

	info := metadata.Current
	if info == nil || info.MigrationCommand == "" {
		return nil
	}

	// Read current config to get current migration_command
	cfg, err := ReadConfig(root)
	if err != nil {
		return nil // Can't read config - skip validation
	}

	currentCommand := GetSnapshotMigrationCommand(cfg.Snapshot)
	if currentCommand == "" {
		// Command was used before but now removed from config
		return fmt.Errorf(`migration_command was removed from config since last snapshot build

  Previous command: %s

Run 'regresql snapshot build' to rebuild the snapshot without migration_command`,
			info.MigrationCommand)
	}

	currentHash := computeCommandHash(currentCommand)
	if currentHash != info.MigrationCommandHash {
		return fmt.Errorf(`migration_command has changed since last snapshot build

  Previous: %s
  Current:  %s

Run 'regresql snapshot build' to rebuild the snapshot`,
			info.MigrationCommand, currentCommand)
	}

	return nil
}

// PlannerSettings is the list of PostgreSQL settings that affect query plans
var PlannerSettings = []string{
	"random_page_cost",
	"seq_page_cost",
	"cpu_tuple_cost",
	"cpu_index_tuple_cost",
	"cpu_operator_cost",
	"effective_cache_size",
	"work_mem",
}

// CaptureServerContext captures PostgreSQL server version and planner settings
func CaptureServerContext(db *sql.DB) (*ServerContext, error) {
	ctx := &ServerContext{
		PlannerSettings: make(map[string]string),
	}

	// Get version info - use current_setting for clean version string
	err := db.QueryRow("SELECT current_setting('server_version'), current_setting('server_version_num')::int").
		Scan(&ctx.Version, &ctx.VersionNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}

	// Get planner settings
	for _, name := range PlannerSettings {
		var value string
		err := db.QueryRow("SELECT current_setting($1)", name).Scan(&value)
		if err != nil {
			continue // setting might not exist in older versions
		}
		ctx.PlannerSettings[name] = value
	}

	return ctx, nil
}

// MajorVersion extracts major version from version_num (e.g., 160002 -> 16)
func (s *ServerContext) MajorVersion() int {
	return s.VersionNum / 10000
}

// ValidateServerContext compares current server context with stored metadata
func ValidateServerContext(db *sql.DB, expected *ServerContext) (*ServerValidation, error) {
	if expected == nil {
		return &ServerValidation{VersionMatch: true}, nil
	}

	current, err := CaptureServerContext(db)
	if err != nil {
		return nil, err
	}

	result := &ServerValidation{
		VersionMatch:  current.VersionNum == expected.VersionNum,
		MajorMismatch: current.MajorVersion() != expected.MajorVersion(),
	}

	if !result.VersionMatch {
		result.VersionDiff = &SettingsDiff{
			Name:     "version",
			Expected: fmt.Sprintf("%s (%d)", expected.Version, expected.VersionNum),
			Actual:   fmt.Sprintf("%s (%d)", current.Version, current.VersionNum),
		}
	}

	// Compare planner settings
	for _, name := range PlannerSettings {
		expectedVal := expected.PlannerSettings[name]
		actualVal := current.PlannerSettings[name]
		if expectedVal != actualVal && expectedVal != "" {
			result.SettingsDiffs = append(result.SettingsDiffs, SettingsDiff{
				Name:     name,
				Expected: expectedVal,
				Actual:   actualVal,
			})
		}
	}

	return result, nil
}

// HasDifferences returns true if there are any version or settings differences
func (v *ServerValidation) HasDifferences() bool {
	return !v.VersionMatch || len(v.SettingsDiffs) > 0
}

// FormatWarning formats the validation result as a warning message
func (v *ServerValidation) FormatWarning() string {
	if !v.HasDifferences() {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Warning: Server settings differ from snapshot creation:\n")

	if v.VersionDiff != nil {
		sb.WriteString(fmt.Sprintf("  PostgreSQL version: %s (snapshot) vs %s (current)\n",
			v.VersionDiff.Expected, v.VersionDiff.Actual))
		if v.MajorMismatch {
			sb.WriteString("  ⚠ Major version mismatch - query behavior may differ\n")
		}
	}

	if len(v.SettingsDiffs) > 0 {
		// Sort for consistent output
		sort.Slice(v.SettingsDiffs, func(i, j int) bool {
			return v.SettingsDiffs[i].Name < v.SettingsDiffs[j].Name
		})
		for _, d := range v.SettingsDiffs {
			sb.WriteString(fmt.Sprintf("  %s: %s (snapshot) vs %s (current)\n",
				d.Name, d.Expected, d.Actual))
		}
	}

	sb.WriteString("\nQuery plans may differ. Use validate_settings: ignore to suppress.")
	return sb.String()
}

// GetValidateSettings returns the validation mode from config (warn, strict, ignore)
func GetValidateSettings(cfg *SnapshotConfig) ValidateSettingsMode {
	if cfg == nil || cfg.ValidateSettings == "" {
		return ValidateSettingsWarn
	}
	switch cfg.ValidateSettings {
	case "strict":
		return ValidateSettingsStrict
	case "ignore":
		return ValidateSettingsIgnore
	default:
		return ValidateSettingsWarn
	}
}
