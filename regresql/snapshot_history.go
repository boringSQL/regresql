package regresql

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TagPattern validates tag names: alphanumeric, hyphen, underscore
var TagPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateTag checks if a tag name is valid
func ValidateTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("tag name cannot be empty")
	}
	if !TagPattern.MatchString(tag) {
		return fmt.Errorf("invalid tag name %q: must contain only alphanumeric characters, hyphens, and underscores", tag)
	}
	return nil
}

// TagSnapshot tags the current snapshot with a version name
func TagSnapshot(snapshotsDir, tag, note, archivePath string) error {
	if err := ValidateTag(tag); err != nil {
		return err
	}

	metadata, err := ReadSnapshotMetadata(snapshotsDir)
	if err != nil {
		return fmt.Errorf("no current snapshot to tag: %w", err)
	}

	if metadata.Current == nil {
		return fmt.Errorf("no current snapshot to tag")
	}

	// Check for duplicate tag in history
	if _, err := GetSnapshotByTag(metadata, tag); err == nil {
		return fmt.Errorf("tag %q already exists", tag)
	}

	// Archive current snapshot if requested
	if archivePath != "" {
		if err := copyFile(metadata.Current.Path, archivePath); err != nil {
			return fmt.Errorf("failed to archive snapshot: %w", err)
		}
		// Update path in current to the archive location
		metadata.Current.Path = archivePath
	}

	// Set tag and note
	metadata.Current.Tag = tag
	if note != "" {
		metadata.Current.Note = note
	}

	return WriteSnapshotMetadataFull(snapshotsDir, metadata)
}

// AddToHistory moves current snapshot to history (for use before overwriting)
func AddToHistory(snapshotsDir, archivePath string) error {
	metadata, err := ReadSnapshotMetadata(snapshotsDir)
	if err != nil {
		return nil // No metadata yet, nothing to archive
	}

	if metadata.Current == nil {
		return nil // No current snapshot to archive
	}

	// Only archive if current has a tag
	if metadata.Current.Tag == "" {
		return nil // Untagged snapshots don't go to history
	}

	// Copy snapshot file if archive path provided
	if archivePath != "" {
		if err := copyFile(metadata.Current.Path, archivePath); err != nil {
			return fmt.Errorf("failed to archive snapshot: %w", err)
		}
		metadata.Current.Path = archivePath
	}

	// Prepend current to history (newest first)
	metadata.History = append([]*SnapshotInfo{metadata.Current}, metadata.History...)
	metadata.Current = nil

	return WriteSnapshotMetadataFull(snapshotsDir, metadata)
}

// GetSnapshotByTag returns snapshot info for a given tag
func GetSnapshotByTag(metadata *SnapshotMetadata, tag string) (*SnapshotInfo, error) {
	if metadata.Current != nil && metadata.Current.Tag == tag {
		return metadata.Current, nil
	}

	for _, info := range metadata.History {
		if info.Tag == tag {
			return info, nil
		}
	}

	return nil, fmt.Errorf("snapshot with tag %q not found", tag)
}

// GetSnapshotByHash returns snapshot info for a given hash prefix
func GetSnapshotByHash(metadata *SnapshotMetadata, hashPrefix string) (*SnapshotInfo, []*SnapshotInfo, error) {
	var matches []*SnapshotInfo

	if metadata.Current != nil && strings.HasPrefix(metadata.Current.Hash, hashPrefix) {
		matches = append(matches, metadata.Current)
	}

	for _, info := range metadata.History {
		if strings.HasPrefix(info.Hash, hashPrefix) {
			matches = append(matches, info)
		}
	}

	if len(matches) == 0 {
		return nil, nil, fmt.Errorf("no snapshot found with hash prefix %q", hashPrefix)
	}

	if len(matches) > 1 {
		return nil, matches, fmt.Errorf("ambiguous hash prefix %q matches %d snapshots", hashPrefix, len(matches))
	}

	return matches[0], nil, nil
}

// ResolveSnapshot resolves a snapshot by tag or hash prefix
func ResolveSnapshot(metadata *SnapshotMetadata, ref string) (*SnapshotInfo, error) {
	// Try tag first
	if info, err := GetSnapshotByTag(metadata, ref); err == nil {
		return info, nil
	}

	// Try hash prefix
	info, ambiguous, err := GetSnapshotByHash(metadata, ref)
	if err != nil {
		if ambiguous != nil {
			var refs []string
			for _, m := range ambiguous {
				refs = append(refs, FormatSnapshotRef(m))
			}
			return nil, fmt.Errorf("ambiguous reference %q matches: %s", ref, strings.Join(refs, ", "))
		}
		return nil, fmt.Errorf("snapshot %q not found (tried as tag and hash prefix)", ref)
	}

	return info, nil
}

// ListSnapshots returns all snapshots (current + history), newest first
func ListSnapshots(metadata *SnapshotMetadata) []*SnapshotInfo {
	var all []*SnapshotInfo
	if metadata.Current != nil {
		all = append(all, metadata.Current)
	}
	all = append(all, metadata.History...)
	return all
}

// IsCurrent returns true if the given info is the current snapshot
func IsCurrent(metadata *SnapshotMetadata, info *SnapshotInfo) bool {
	return metadata.Current != nil && metadata.Current.Hash == info.Hash
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	// Create destination directory if needed
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// SnapshotExists checks if a snapshot file exists
func SnapshotExists(info *SnapshotInfo) bool {
	if info == nil || info.Path == "" {
		return false
	}
	_, err := os.Stat(info.Path)
	return err == nil
}

// FormatSnapshotRef returns a human-readable reference for a snapshot
func FormatSnapshotRef(info *SnapshotInfo) string {
	if info.Tag != "" {
		return info.Tag
	}
	return TruncateHash(info.Hash)
}
