package regresql

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		format  SnapshotFormat
		wantErr bool
	}{
		{FormatCustom, false},
		{FormatPlain, false},
		{FormatDirectory, false},
		{SnapshotFormat("invalid"), true},
		{SnapshotFormat(""), true},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			err := validateFormat(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFormat(%q) error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSection(t *testing.T) {
	tests := []struct {
		section string
		wantErr bool
	}{
		{"pre-data", false},
		{"data", false},
		{"post-data", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.section, func(t *testing.T) {
			err := validateSection(tt.section)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSection(%q) error = %v, wantErr %v", tt.section, err, tt.wantErr)
			}
		})
	}
}

func TestBuildPgDumpArgs(t *testing.T) {
	tests := []struct {
		name string
		opts SnapshotOptions
		want []string
	}{
		{
			name: "custom format",
			opts: SnapshotOptions{
				OutputPath: "/tmp/test.dump",
				Format:     FormatCustom,
			},
			want: []string{
				"--dbname", "postgres://test",
				"--format=custom", "--file", "/tmp/test.dump",
			},
		},
		{
			name: "plain format",
			opts: SnapshotOptions{
				OutputPath: "/tmp/test.sql",
				Format:     FormatPlain,
			},
			want: []string{
				"--dbname", "postgres://test",
				"--format=plain",
			},
		},
		{
			name: "directory format",
			opts: SnapshotOptions{
				OutputPath: "/tmp/testdir",
				Format:     FormatDirectory,
			},
			want: []string{
				"--dbname", "postgres://test",
				"--format=directory", "--file", "/tmp/testdir",
			},
		},
		{
			name: "schema only",
			opts: SnapshotOptions{
				OutputPath: "/tmp/test.dump",
				Format:     FormatCustom,
				SchemaOnly: true,
			},
			want: []string{
				"--dbname", "postgres://test",
				"--format=custom", "--file", "/tmp/test.dump",
				"--schema-only",
			},
		},
		{
			name: "with section",
			opts: SnapshotOptions{
				OutputPath: "/tmp/test.sql",
				Format:     FormatPlain,
				Section:    "pre-data",
			},
			want: []string{
				"--dbname", "postgres://test",
				"--format=plain",
				"--section", "pre-data",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPgDumpArgs("postgres://test", tt.opts)
			if len(got) != len(tt.want) {
				t.Errorf("buildPgDumpArgs() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildPgDumpArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestComputeSingleFileHash(t *testing.T) {
	// Create a temp file with known content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	hash, err := computeSingleFileHash(testFile)
	if err != nil {
		t.Fatalf("computeSingleFileHash() error = %v", err)
	}

	// SHA256 of "hello world" is known
	expectedPrefix := "sha256:"
	if len(hash) < len(expectedPrefix) || hash[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("hash should start with %q, got %q", expectedPrefix, hash)
	}

	// Hash should be deterministic
	hash2, err := computeSingleFileHash(testFile)
	if err != nil {
		t.Fatalf("computeSingleFileHash() second call error = %v", err)
	}
	if hash != hash2 {
		t.Errorf("hash should be deterministic, got %q and %q", hash, hash2)
	}
}

func TestComputeDirectoryHash(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory structure
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	hash, err := computeDirectoryHash(tmpDir)
	if err != nil {
		t.Fatalf("computeDirectoryHash() error = %v", err)
	}

	expectedPrefix := "sha256:"
	if len(hash) < len(expectedPrefix) || hash[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("hash should start with %q, got %q", expectedPrefix, hash)
	}

	// Hash should be deterministic
	hash2, err := computeDirectoryHash(tmpDir)
	if err != nil {
		t.Fatalf("computeDirectoryHash() second call error = %v", err)
	}
	if hash != hash2 {
		t.Errorf("hash should be deterministic, got %q and %q", hash, hash2)
	}
}

func TestSnapshotMetadataRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()

	info := &SnapshotInfo{
		Path:      "snapshots/test.dump",
		Hash:      "sha256:abc123",
		SizeBytes: 12345,
		Format:    "custom",
	}

	// Write metadata
	if err := WriteSnapshotMetadata(tmpDir, info); err != nil {
		t.Fatalf("WriteSnapshotMetadata() error = %v", err)
	}

	// Read metadata back
	metadata, err := ReadSnapshotMetadata(tmpDir)
	if err != nil {
		t.Fatalf("ReadSnapshotMetadata() error = %v", err)
	}

	if metadata.Current == nil {
		t.Fatal("metadata.Current should not be nil")
	}

	if metadata.Current.Path != info.Path {
		t.Errorf("Path = %q, want %q", metadata.Current.Path, info.Path)
	}
	if metadata.Current.Hash != info.Hash {
		t.Errorf("Hash = %q, want %q", metadata.Current.Hash, info.Hash)
	}
	if metadata.Current.SizeBytes != info.SizeBytes {
		t.Errorf("SizeBytes = %d, want %d", metadata.Current.SizeBytes, info.SizeBytes)
	}
	if metadata.Current.Format != info.Format {
		t.Errorf("Format = %q, want %q", metadata.Current.Format, info.Format)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestGetSnapshotPath(t *testing.T) {
	tests := []struct {
		name string
		cfg  *SnapshotConfig
		root string
		want string
	}{
		{
			name: "nil config",
			cfg:  nil,
			root: "/project",
			want: "/project/snapshots/default.dump",
		},
		{
			name: "empty config",
			cfg:  &SnapshotConfig{},
			root: "/project",
			want: "/project/snapshots/default.dump",
		},
		{
			name: "relative path in config",
			cfg:  &SnapshotConfig{Path: "snapshots/custom.dump"},
			root: "/project",
			want: "/project/snapshots/custom.dump",
		},
		{
			name: "absolute path in config",
			cfg:  &SnapshotConfig{Path: "/absolute/path/snapshot.dump"},
			root: "/project",
			want: "/absolute/path/snapshot.dump",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSnapshotPath(tt.cfg, tt.root)
			if got != tt.want {
				t.Errorf("GetSnapshotPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetSnapshotFormat(t *testing.T) {
	tests := []struct {
		name string
		cfg  *SnapshotConfig
		want SnapshotFormat
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: DefaultSnapshotFormat,
		},
		{
			name: "empty config",
			cfg:  &SnapshotConfig{},
			want: DefaultSnapshotFormat,
		},
		{
			name: "custom format",
			cfg:  &SnapshotConfig{Format: "custom"},
			want: FormatCustom,
		},
		{
			name: "plain format",
			cfg:  &SnapshotConfig{Format: "plain"},
			want: FormatPlain,
		},
		{
			name: "directory format uppercase",
			cfg:  &SnapshotConfig{Format: "DIRECTORY"},
			want: FormatDirectory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSnapshotFormat(tt.cfg)
			if got != tt.want {
				t.Errorf("GetSnapshotFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectSnapshotFormat(t *testing.T) {
	tmpDir := t.TempDir()

	// create test files
	sqlFile := filepath.Join(tmpDir, "test.sql")
	if err := os.WriteFile(sqlFile, []byte("SELECT 1;"), 0644); err != nil {
		t.Fatalf("failed to create sql file: %v", err)
	}

	dumpFile := filepath.Join(tmpDir, "test.dump")
	if err := os.WriteFile(dumpFile, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create dump file: %v", err)
	}

	dirSnapshot := filepath.Join(tmpDir, "snapshot_dir")
	if err := os.Mkdir(dirSnapshot, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	tests := []struct {
		name string
		path string
		want SnapshotFormat
	}{
		{"sql file", sqlFile, FormatPlain},
		{"dump file", dumpFile, FormatCustom},
		{"directory", dirSnapshot, FormatDirectory},
		{"nonexistent", "/nonexistent/path", FormatCustom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectSnapshotFormat(tt.path)
			if got != tt.want {
				t.Errorf("detectSnapshotFormat(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRestoreSnapshotMissingFile(t *testing.T) {
	opts := RestoreOptions{
		InputPath: "/nonexistent/snapshot.dump",
	}

	err := RestoreSnapshot("postgres://test", opts)
	if err == nil {
		t.Error("RestoreSnapshot() should return error for missing file")
	}
}
