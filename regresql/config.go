package regresql

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type (
	config struct {
		Extends        string                `yaml:"extends,omitempty"`
		Root           string                `yaml:"root"`
		PgUri          string                `yaml:"pguri"`
		Timeout        string                `yaml:"timeout,omitempty"` // statement_timeout, e.g. "30s"
		Ignore         []string              `yaml:"ignore,omitempty"`
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
		Path             string   `yaml:"path,omitempty"`
		Format           string   `yaml:"format,omitempty"`
		Schema           string   `yaml:"schema,omitempty"`
		Migrations       string   `yaml:"migrations,omitempty"`
		MigrationCommand string   `yaml:"migration_command,omitempty"`
		Fixtures         []string `yaml:"fixtures,omitempty"`
		Fixturize        []string `yaml:"fixturize,omitempty"`
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
	return loadConfig(s.getRegressConfigFile())
}

// ReadConfig reads the configuration from the regress.yaml file
func ReadConfig(root string) (config, error) {
	cfg, err := ReadConfigFile(root)
	if err != nil {
		return cfg, err
	}
	if uri := os.Getenv("DATABASE_URL"); uri != "" {
		cfg.PgUri = uri
	}
	return cfg, nil
}

// ReadConfigFile reads regress.yaml as written, without the DATABASE_URL
// override. Use it for `config get`/`config set`, which operate on the file.
func ReadConfigFile(root string) (config, error) {
	return loadConfig(filepath.Join(root, "regresql", "regress.yaml"))
}

func loadConfig(configFile string) (config, error) {
	var cfg config

	data, err := os.ReadFile(configFile)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config '%s': %w", configFile, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config '%s': %w", configFile, err)
	}

	if cfg.Extends != "" {
		base, err := resolveAndLoadPack(cfg.Extends, filepath.Dir(configFile))
		if err != nil {
			return config{}, fmt.Errorf("failed to resolve extends %q: %w", cfg.Extends, err)
		}
		cfg.Extends = ""
		cfg = mergeConfig(base, cfg)
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

func GetCriticalTables() []string {
	if cachedConfig == nil || cachedConfig.Policies == nil {
		return nil
	}
	return cachedConfig.Policies.CriticalTables
}

func GetPoliciesConfig() *PoliciesConfig {
	if cachedConfig == nil {
		return nil
	}
	return cachedConfig.Policies
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
			QErrorRatio:          2.0,
			QErrorFloor:          10.0,
		}
	}
	cfg := cachedConfig.Analyze
	result := &AnalyzeConfig{
		Enabled:              cfg.Enabled,
		Comparison:           cfg.Comparison,
		BufferThreshold:      cfg.BufferThreshold,
		CostThreshold:        cfg.CostThreshold,
		ImprovementThreshold: cfg.ImprovementThreshold,
		QErrorRatio:          cfg.QErrorRatio,
		QErrorFloor:          cfg.QErrorFloor,
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
	if result.QErrorRatio == 0 {
		result.QErrorRatio = 2.0
	}
	if result.QErrorFloor == 0 {
		result.QErrorFloor = 10.0
	}
	return result
}

func mergeConfig(base, over config) config {
	out := base
	if over.Root != "" {
		out.Root = over.Root
	}
	if over.PgUri != "" {
		out.PgUri = over.PgUri
	}
	if over.Timeout != "" {
		out.Timeout = over.Timeout
	}
	out.Ignore = mergeStringSlice(base.Ignore, over.Ignore)
	out.PlanQuality = mergePlanQuality(base.PlanQuality, over.PlanQuality)
	out.DiffComparison = mergeDiffComparison(base.DiffComparison, over.DiffComparison)
	out.Snapshot = mergeSnapshotConfig(base.Snapshot, over.Snapshot)
	out.Analyze = mergeAnalyzeConfig(base.Analyze, over.Analyze)
	out.Stats = mergeStatsConfig(base.Stats, over.Stats)
	out.Policies = mergePoliciesConfig(base.Policies, over.Policies)
	return out
}

func mergeStringSlice(a, b []string) []string {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, v := range a {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, v := range b {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func mergeStringMap(a, b map[string]string) map[string]string {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func mergePlanQuality(a, b *PlanQualityGlobal) *PlanQualityGlobal {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return &PlanQualityGlobal{
		IgnoreSeqScanTables: mergeStringSlice(a.IgnoreSeqScanTables, b.IgnoreSeqScanTables),
	}
}

func mergePoliciesConfig(a, b *PoliciesConfig) *PoliciesConfig {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return &PoliciesConfig{
		CriticalTables: mergeStringSlice(a.CriticalTables, b.CriticalTables),
		Severity:       mergeStringMap(a.Severity, b.Severity),
		Reasons:        mergeStringMap(a.Reasons, b.Reasons),
	}
}

func mergeDiffComparison(a, b *DiffComparisonGlobal) *DiffComparisonGlobal {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := *a
	if b.FloatTolerance != 0 {
		out.FloatTolerance = b.FloatTolerance
	}
	if b.MaxSamples != 0 {
		out.MaxSamples = b.MaxSamples
	}
	return &out
}

func mergeSnapshotConfig(a, b *SnapshotConfig) *SnapshotConfig {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := *a
	if b.Path != "" {
		out.Path = b.Path
	}
	if b.Format != "" {
		out.Format = b.Format
	}
	if b.Schema != "" {
		out.Schema = b.Schema
	}
	if b.Migrations != "" {
		out.Migrations = b.Migrations
	}
	if b.MigrationCommand != "" {
		out.MigrationCommand = b.MigrationCommand
	}
	out.Fixtures = mergeStringSlice(a.Fixtures, b.Fixtures)
	out.Fixturize = mergeStringSlice(a.Fixturize, b.Fixturize)
	if b.RestoreDatabase != "" {
		out.RestoreDatabase = b.RestoreDatabase
	}
	if b.ValidateSettings != "" {
		out.ValidateSettings = b.ValidateSettings
	}
	return &out
}

func mergeAnalyzeConfig(a, b *AnalyzeConfig) *AnalyzeConfig {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := *a
	if b.Enabled {
		out.Enabled = true
	}
	if b.Comparison != "" {
		out.Comparison = b.Comparison
	}
	if b.BufferThreshold != 0 {
		out.BufferThreshold = b.BufferThreshold
	}
	if b.CostThreshold != 0 {
		out.CostThreshold = b.CostThreshold
	}
	if b.ImprovementThreshold != 0 {
		out.ImprovementThreshold = b.ImprovementThreshold
	}
	if b.QErrorRatio != 0 {
		out.QErrorRatio = b.QErrorRatio
	}
	if b.QErrorFloor != 0 {
		out.QErrorFloor = b.QErrorFloor
	}
	return &out
}

func mergeStatsConfig(a, b *StatsConfig) *StatsConfig {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := *a
	if b.Default != "" {
		out.Default = b.Default
	}
	return &out
}

// GetStatementTimeout returns the default statement_timeout (0 = none).
func GetStatementTimeout() time.Duration {
	if cachedConfig == nil || cachedConfig.Timeout == "" {
		return 0
	}
	d, err := time.ParseDuration(cachedConfig.Timeout)
	if err != nil {
		return 0
	}
	return d
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

func GetQErrorRatio() float64 {
	return GetAnalyzeConfig().QErrorRatio
}

func GetQErrorFloor() float64 {
	return GetAnalyzeConfig().QErrorFloor
}
