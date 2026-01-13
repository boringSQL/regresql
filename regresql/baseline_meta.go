package regresql

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const BaselineMetadataFile = ".regresql-meta.yaml"

type (
	// BaselineMetadata tracks snapshot correlation for baselines
	BaselineMetadata struct {
		Baselines map[string]*BaselineInfo `yaml:"baselines"`
	}

	// BaselineInfo records when a baseline was created/updated and against which snapshot
	BaselineInfo struct {
		SnapshotTag  string    `yaml:"snapshot_tag,omitempty"`
		SnapshotHash string    `yaml:"snapshot_hash"`
		Created      time.Time `yaml:"created"`
		Updated      time.Time `yaml:"updated,omitempty"`
		Note         string    `yaml:"note,omitempty"`
	}
)

func LoadBaselineMetadata(expectedDir string) (*BaselineMetadata, error) {
	metaPath := filepath.Join(expectedDir, BaselineMetadataFile)

	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &BaselineMetadata{Baselines: make(map[string]*BaselineInfo)}, nil
		}
		return nil, err
	}

	var meta BaselineMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	if meta.Baselines == nil {
		meta.Baselines = make(map[string]*BaselineInfo)
	}

	return &meta, nil
}

func SaveBaselineMetadata(expectedDir string, meta *BaselineMetadata) error {
	metaPath := filepath.Join(expectedDir, BaselineMetadataFile)

	if err := os.MkdirAll(expectedDir, 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(meta)
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0o644)
}

func RecordBaselineUpdate(expectedDir, baselinePath string, snapshot *SnapshotInfo, note string) error {
	meta, err := LoadBaselineMetadata(expectedDir)
	if err != nil {
		return err
	}

	relPath, err := filepath.Rel(expectedDir, baselinePath)
	if err != nil {
		relPath = baselinePath
	}

	now := time.Now().UTC()

	info, exists := meta.Baselines[relPath]
	if !exists {
		info = &BaselineInfo{
			Created: now,
		}
		meta.Baselines[relPath] = info
	}

	info.Updated = now
	if note != "" {
		info.Note = note
	}

	if snapshot != nil {
		info.SnapshotTag = snapshot.Tag
		info.SnapshotHash = snapshot.Hash
	}

	return SaveBaselineMetadata(expectedDir, meta)
}

func GetBaselineInfo(expectedDir, baselinePath string) (*BaselineInfo, error) {
	meta, err := LoadBaselineMetadata(expectedDir)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(expectedDir, baselinePath)
	if err != nil {
		relPath = baselinePath
	}

	info, exists := meta.Baselines[relPath]
	if !exists {
		return nil, nil
	}

	return info, nil
}

func GroupBaselinesBySnapshot(meta *BaselineMetadata) map[string][]string {
	groups := make(map[string][]string)

	for path, info := range meta.Baselines {
		key := info.SnapshotHash
		if info.SnapshotTag != "" {
			key = info.SnapshotTag + " (" + TruncateHash(info.SnapshotHash) + ")"
		} else if len(key) > 20 {
			key = TruncateHash(key)
		}

		groups[key] = append(groups[key], path)
	}

	return groups
}

func CheckBaselineCorrelation(meta *BaselineMetadata, currentSnapshot *SnapshotInfo) (matched, outdated []string) {
	for path, info := range meta.Baselines {
		if currentSnapshot != nil && info.SnapshotHash == currentSnapshot.Hash {
			matched = append(matched, path)
		} else {
			outdated = append(outdated, path)
		}
	}
	return
}

// TruncateHash returns a shortened hash for display
func TruncateHash(hash string) string {
	if len(hash) > 20 {
		return hash[:20] + "..."
	}
	return hash
}
