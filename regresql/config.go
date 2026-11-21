package regresql

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

type (
	Config struct {
		Extends        string                `yaml:"extends,omitempty"`
		Root           string                `yaml:"root"`
		PgUri          string                `yaml:"pguri"`
		Timeout        string                `yaml:"timeout,omitempty"` // statement_timeout, e.g. "30s"
		Ignore         mergeAppend           `yaml:"ignore,omitempty"`
		PlanQuality    *PlanQualityGlobal    `yaml:"plan_quality,omitempty"`
		DiffComparison *DiffComparisonGlobal `yaml:"diff_comparison,omitempty"`
		Snapshot       *SnapshotConfig       `yaml:"snapshot,omitempty"`
		Analyze        *AnalyzeConfig        `yaml:"analyze,omitempty"`
		Stats          *StatsConfig          `yaml:"stats,omitempty"`
		Policies       *PoliciesConfig       `yaml:"policies,omitempty"`
	}

	StatsConfig struct {
		Default string `yaml:"default,omitempty"`
	}

	AnalyzeConfig struct {
		Enabled              bool    `yaml:"enabled"`
		Comparison           string  `yaml:"comparison,omitempty"`            // "auto" | "cost" | "buffers"
		BufferThreshold      float64 `yaml:"buffer_threshold,omitempty"`      // default: 2.0
		CostThreshold        float64 `yaml:"cost_threshold,omitempty"`        // default: 10.0
		ImprovementThreshold float64 `yaml:"improvement_threshold,omitempty"` // default: 20.0
		QErrorRatio          float64 `yaml:"qerror_ratio,omitempty"`          // default: 2.0 (x worse than baseline)
		QErrorFloor          float64 `yaml:"qerror_floor,omitempty"`          // default: 10.0 (absolute q-error floor)
	}

	PlanQualityGlobal struct {
		IgnoreSeqScanTables []string `yaml:"ignore_seqscan_tables,omitempty"`
	}

	PoliciesConfig struct {
		CriticalTables []string          `yaml:"critical_tables,omitempty"`
		Severity       map[string]string `yaml:"severity,omitempty"`
		Reasons        map[string]string `yaml:"reasons,omitempty"`
	}

	DiffComparisonGlobal struct {
		FloatTolerance float64 `yaml:"float_tolerance,omitempty"`
		MaxSamples     int     `yaml:"max_samples,omitempty"`
	}

	SnapshotConfig struct {
		Path             string      `yaml:"path,omitempty"`
		Format           string      `yaml:"format,omitempty"`
		Schema           string      `yaml:"schema,omitempty"`
		Migrations       string      `yaml:"migrations,omitempty"`
		MigrationCommand string      `yaml:"migration_command,omitempty"`
		Fixtures         mergeAppend `yaml:"fixtures,omitempty"`
		Fixturize        mergeAppend `yaml:"fixturize,omitempty"`
		RestoreDatabase  string      `yaml:"restore_database,omitempty"`
		ValidateSettings string      `yaml:"validate_settings,omitempty"`
	}
)

// mergeAppend appends list values across extends instead of replacing them,
// deduplicating on the way.
type mergeAppend []string

func (m *mergeAppend) UnmarshalYAML(value *yaml.Node) error {
	var values []string
	if err := value.Decode(&values); err != nil {
		return err
	}
	for _, v := range values {
		if !slices.Contains(*m, v) {
			*m = append(*m, v)
		}
	}
	return nil
}

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

	cfg := Config{
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

func (s *Suite) readConfig() (Config, error) {
	cfg, err := loadConfig(s.getRegressConfigFile())
	if err != nil {
		return cfg, err
	}
	return withEnvOverride(cfg), nil
}

// ReadConfig reads the configuration from the regress.yaml file
func ReadConfig(root string) (Config, error) {
	cfg, err := ReadConfigFile(root)
	if err != nil {
		return cfg, err
	}
	return withEnvOverride(cfg), nil
}

// withEnvOverride lets DATABASE_URL override the configured pguri at run time.
func withEnvOverride(cfg Config) Config {
	if uri := os.Getenv("DATABASE_URL"); uri != "" {
		cfg.PgUri = uri
	}
	return cfg
}

// ReadConfigFile reads regress.yaml as written, without the DATABASE_URL
// override. Use it for `config get`/`config set`, which operate on the file.
func ReadConfigFile(root string) (Config, error) {
	return loadConfig(filepath.Join(root, "regresql", "regress.yaml"))
}

func loadConfig(configFile string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(configFile)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, fmt.Errorf("no regresql config at '%s': run 'regresql init postgres://...' first, or pass -C <dir>", configFile)
	}
	if err != nil {
		return cfg, fmt.Errorf("failed to read config '%s': %w", configFile, err)
	}

	var head struct {
		Extends string `yaml:"extends"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return cfg, fmt.Errorf("failed to parse config '%s': %w", configFile, err)
	}

	// Decode the pack first, then the project file over it: scalars and maps
	// merge per key, lists are replaced (see mergeAppend).
	if head.Extends != "" {
		pack, source, err := readPack(head.Extends, filepath.Dir(configFile))
		if err != nil {
			return Config{}, fmt.Errorf("failed to resolve extends %q: %w", head.Extends, err)
		}
		if err := yaml.Unmarshal(pack, &cfg); err != nil {
			return Config{}, fmt.Errorf("failed to parse pack %s: %w", source, err)
		}
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse config '%s': %w", configFile, err)
	}
	// Chained extends in packs is not supported in v1.
	cfg.Extends = ""

	if cfg.Policies != nil {
		for rule, value := range cfg.Policies.Severity {
			if _, err := ParseSeverity(value); err != nil {
				return Config{}, fmt.Errorf("invalid config '%s': policies.severity.%s: %w", configFile, rule, err)
			}
		}
	}

	return cfg, nil
}

// UpdateConfigField updates a single field in the config file, preserving other values
func UpdateConfigField(root, key, value string) error {
	configFile := filepath.Join(root, "regresql", "regress.yaml")

	cfg, err := ReadConfigFile(root)
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

func (c Config) AnalyzeConfig() AnalyzeConfig {
	if c.Analyze != nil {
		return *c.Analyze
	}
	return defaultAnalyzeConfig
}

func (c Config) DiffConfig() *DiffConfig {
	cfg := DefaultDiffConfig()
	if d := c.DiffComparison; d != nil {
		if d.FloatTolerance > 0 {
			cfg.FloatTolerance = d.FloatTolerance
		}
		if d.MaxSamples > 0 {
			cfg.MaxSamples = d.MaxSamples
		}
	}
	return cfg
}

func (c Config) IgnoredSeqScanTables() []string {
	if c.PlanQuality == nil {
		return nil
	}
	return c.PlanQuality.IgnoreSeqScanTables
}

func (c Config) CriticalTables() []string {
	if c.Policies == nil {
		return nil
	}
	return c.Policies.CriticalTables
}

var defaultAnalyzeConfig = AnalyzeConfig{
	Comparison:           "auto",
	BufferThreshold:      2.0,
	CostThreshold:        10.0,
	ImprovementThreshold: 20.0,
	QErrorRatio:          2.0,
	QErrorFloor:          10.0,
}

// UnmarshalYAML starts a fresh section from the defaults, so a key left out
// keeps its default while an explicit 0 stays 0. A section that already holds
// values (a pack decoded earlier) is layered over instead.
func (a *AnalyzeConfig) UnmarshalYAML(value *yaml.Node) error {
	if *a == (AnalyzeConfig{}) {
		*a = defaultAnalyzeConfig
	}
	type plain AnalyzeConfig
	return value.Decode((*plain)(a))
}
