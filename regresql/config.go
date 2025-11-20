package regresql

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config structure is useful to store the PostgreSQL connection string, and
// also remember the code root directory, which as of now is always either
// ./ or the -C command line parameter.
type (
	config struct {
		Root           string                 `yaml:"root"`
		PgUri          string                 `yaml:"pguri"`
		UseFixtures    bool                   `yaml:"use_fixtures,omitempty"`
		PlanQuality    *PlanQualityGlobal     `yaml:"plan_quality,omitempty"`
		DiffComparison *DiffComparisonGlobal  `yaml:"diff_comparison,omitempty"`
	}

	PlanQualityGlobal struct {
		IgnoreSeqScanTables []string `yaml:"ignore_seqscan_tables,omitempty"`
	}

	DiffComparisonGlobal struct {
		FloatTolerance float64 `yaml:"float_tolerance,omitempty"`
		MaxSamples     int     `yaml:"max_samples,omitempty"`
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

func (s *Suite) setupConfig(pguri string, useFixtures bool) {
	configFile := s.getRegressConfigFile()

	cfg := config{
		Root:  s.Root,
		PgUri: pguri,
	}
	if useFixtures {
		cfg.UseFixtures = true
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
