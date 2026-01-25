package regresql

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type (
	config struct {
		Root           string                `yaml:"root"`
		PgUri          string                `yaml:"pguri"`
		Ignore         []string              `yaml:"ignore,omitempty"`
		PlanQuality    *PlanQualityGlobal    `yaml:"plan_quality,omitempty"`
		DiffComparison *DiffComparisonGlobal `yaml:"diff_comparison,omitempty"`
		Snapshot       *SnapshotConfig       `yaml:"snapshot,omitempty"`
		Analyze        *AnalyzeConfig        `yaml:"analyze,omitempty"`
	}

	AnalyzeConfig struct {
		Enabled              bool    `yaml:"enabled"`
		Comparison           string  `yaml:"comparison,omitempty"`            // "auto" | "cost" | "buffers"
		BufferThreshold      float64 `yaml:"buffer_threshold,omitempty"`      // default: 2.0
		CostThreshold        float64 `yaml:"cost_threshold,omitempty"`        // default: 10.0
		ImprovementThreshold float64 `yaml:"improvement_threshold,omitempty"` // default: 20.0
	}

	PlanQualityGlobal struct {
		IgnoreSeqScanTables []string `yaml:"ignore_seqscan_tables,omitempty"`
	}

	DiffComparisonGlobal struct {
		FloatTolerance float64 `yaml:"float_tolerance,omitempty"`
		MaxSamples     int     `yaml:"max_samples,omitempty"`
	}

	SnapshotConfig struct {
		Path             string   `yaml:"path,omitempty"`
		Format           string   `yaml:"format,omitempty"`
		Schema           string   `yaml:"schema,omitempty"`
		Migrations       string   `yaml:"migrations,omitempty"`
		MigrationCommand string   `yaml:"migration_command,omitempty"`
		Fixtures         []string `yaml:"fixtures,omitempty"`
		RestoreDatabase  string   `yaml:"restore_database,omitempty"`
		ValidateSettings string   `yaml:"validate_settings,omitempty"`
	}
)

func (s *Suite) getRegressConfigFile() string {
	return filepath.Join(s.RegressDir, "regress.yaml")
}

func (s *Suite) createRegressDir() error {
	stat, err := os.Stat(s.RegressDir)
	if err != nil || !stat.IsDir() {
		fmt.Printf("Creating directory '%s'\n", s.RegressDir)
		err := os.Mkdir(s.RegressDir, 0o755)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Directory '%s' already exists\n", s.RegressDir)
	}
	return nil
}

func (s *Suite) setupConfig(pguri string) {
	configFile := s.getRegressConfigFile()

	cfg := config{
		Root:  s.Root,
		PgUri: pguri,
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		fmt.Printf("Error marshaling config to YAML: %s\n", err)
		return
	}

	fmt.Printf("Creating configuration file '%s'\n", configFile)
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		fmt.Printf("Error writing config file '%s': %s\n", configFile, err)
	}
}

func (s *Suite) readConfig() (config, error) {
	var cfg config
	configFile := s.getRegressConfigFile()

	data, err := os.ReadFile(configFile)
	if err != nil {
		return cfg, fmt.Errorf("Failed to read config '%s': %s",
			configFile,
			err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("Failed to parse config '%s': %s",
			configFile,
			err)
	}

	return cfg, nil
}

// ReadConfig reads the configuration from the regress.yaml file
func ReadConfig(root string) (config, error) {
	var cfg config
	configFile := filepath.Join(root, "regresql", "regress.yaml")

	data, err := os.ReadFile(configFile)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config '%s': %w", configFile, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config '%s': %w", configFile, err)
	}

	return cfg, nil
}

// UpdateConfigField updates a single field in the config file, preserving other values
func UpdateConfigField(root, key, value string) error {
	configFile := filepath.Join(root, "regresql", "regress.yaml")

	cfg, err := ReadConfig(root)
	if err != nil {
		return err
	}

	switch key {
	case "pguri":
		cfg.PgUri = value
	default:
		return fmt.Errorf("unsupported config key: %s", key)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

var cachedConfig *config

func SetGlobalConfig(cfg config) {
	cachedConfig = &cfg
}

func GetIgnoredSeqScanTables() []string {
	if cachedConfig == nil || cachedConfig.PlanQuality == nil {
		return nil
	}
	return cachedConfig.PlanQuality.IgnoreSeqScanTables
}

func GetDiffConfig() *DiffConfig {
	cfg := DefaultDiffConfig()
	if cachedConfig != nil && cachedConfig.DiffComparison != nil {
		dc := cachedConfig.DiffComparison
		if dc.FloatTolerance > 0 {
			cfg.FloatTolerance = dc.FloatTolerance
		}
		if dc.MaxSamples > 0 {
			cfg.MaxSamples = dc.MaxSamples
		}
	}
	return cfg
}

func GetAnalyzeConfig() *AnalyzeConfig {
	if cachedConfig == nil || cachedConfig.Analyze == nil {
		return &AnalyzeConfig{
			Enabled:              false,
			Comparison:           "auto",
			BufferThreshold:      2.0,
			CostThreshold:        10.0,
			ImprovementThreshold: 20.0,
		}
	}
	cfg := cachedConfig.Analyze
	result := &AnalyzeConfig{
		Enabled:              cfg.Enabled,
		Comparison:           cfg.Comparison,
		BufferThreshold:      cfg.BufferThreshold,
		CostThreshold:        cfg.CostThreshold,
		ImprovementThreshold: cfg.ImprovementThreshold,
	}
	if result.Comparison == "" {
		result.Comparison = "auto"
	}
	if result.BufferThreshold == 0 {
		result.BufferThreshold = 2.0
	}
	if result.CostThreshold == 0 {
		result.CostThreshold = 10.0
	}
	if result.ImprovementThreshold == 0 {
		result.ImprovementThreshold = 20.0
	}
	return result
}

func IsAnalyzeEnabled() bool {
	return GetAnalyzeConfig().Enabled
}

func GetBufferThreshold() float64 {
	return GetAnalyzeConfig().BufferThreshold
}

func GetCostThreshold() float64 {
	return GetAnalyzeConfig().CostThreshold
}

func GetComparisonMode() string {
	return GetAnalyzeConfig().Comparison
}

func GetImprovementThreshold() float64 {
	return GetAnalyzeConfig().ImprovementThreshold
}
