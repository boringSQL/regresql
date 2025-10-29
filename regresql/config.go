package regresql

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	// "github.com/spf13/viper"
	"github.com/theherk/viper" // fork with write support
)

// Config structure is useful to store the PostgreSQL connection string, and
// also remember the code root directory, which as of now is always either
// ./ or the -C command line parameter.
type config struct {
	Root        string
	PgUri       string
	UseFixtures bool               `yaml:"use_fixtures"`
	PlanQuality *PlanQualityGlobal `yaml:"plan_quality,omitempty"`
}

type PlanQualityGlobal struct {
	IgnoreSeqScanTables []string `yaml:"ignore_seqscan_tables,omitempty"`
}

func (s *Suite) getRegressConfigFile() string {
	return filepath.Join(s.RegressDir, "regress.yaml")
}

func (s *Suite) createRegressDir() error {
	stat, err := os.Stat(s.RegressDir)
	if err != nil || !stat.IsDir() {
		fmt.Printf("Creating directory '%s'\n", s.RegressDir)
		err := os.Mkdir(s.RegressDir, 0755)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Directory '%s' already exists\n", s.RegressDir)
	}
	return nil
}

func (s *Suite) setupConfig(pguri string, useFixtures bool) {
	v := viper.New()
	configFile := s.getRegressConfigFile()

	v.Set("Root", s.Root)
	v.Set("pguri", pguri)
	if useFixtures {
		v.Set("use_fixtures", true)
	}

	fmt.Printf("Creating configuration file '%s'\n", configFile)
	v.WriteConfigAs(configFile)
}

func (s *Suite) readConfig() (config, error) {
	var config config
	v := viper.New()
	v.SetConfigType("yaml")
	configFile := s.getRegressConfigFile()

	data, err := ioutil.ReadFile(configFile)

	if err != nil {
		return config, fmt.Errorf("Failed to read config '%s': %s",
			configFile,
			err)
	}

	v.ReadConfig(bytes.NewBuffer(data))
	v.Unmarshal(&config)

	return config, nil
}

// ReadConfig reads the configuration from the regress.yaml file
func ReadConfig(root string) (config, error) {
	var cfg config
	v := viper.New()
	v.SetConfigType("yaml")
	configFile := filepath.Join(root, "regresql", "regress.yaml")

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config '%s': %w", configFile, err)
	}

	v.ReadConfig(bytes.NewBuffer(data))
	v.Unmarshal(&cfg)

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
