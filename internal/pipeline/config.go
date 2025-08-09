package pipeline

import (
	"fmt"
	"os"
	"regexp"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

// TODO validate
type PipelineConfig struct {
	PipelineName   string `yaml:"name"`
	EntryPoint     string `yaml:"entry_point" default:"index.js"`
	SourceSuffix   string `yaml:"source_suffix"`
	TargetSuffix   string `yaml:"target_suffix"`
	SourceSuffixRe *regexp.Regexp `yaml:"-"`
}

func loadConfig(configFile string) (*PipelineConfig, error) {
	config := &PipelineConfig{}

	file, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if err := defaults.Set(config); err != nil {
		return nil, err
	}

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	config.SourceSuffixRe, err = regexp.Compile(fmt.Sprintf(`\.(%s)\b`, config.SourceSuffix))
	if err != nil {
		return nil, fmt.Errorf("failed to compile source_suffix pattern: %s", err)
	}
	return config, nil
}
