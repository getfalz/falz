package pipeline

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type PipelineConfig struct {
	PipelineName   string `yaml:"name"`
	EntryPoint     string `yaml:"entry_point"`
	SourceSuffix   string `yaml:"source_suffix"`
	TargetSuffix   string `yaml:"target_suffix"`
	SourceSuffixRe *regexp.Regexp
}

func loadConfig(configFile string) (*PipelineConfig, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config PipelineConfig

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	config.SourceSuffixRe, err = regexp.Compile(fmt.Sprintf(`\.(%s)\b`, config.SourceSuffix))
	if err != nil {
		return nil, fmt.Errorf("failed to compile source_suffix pattern: %s", err)
	}
	return &config, nil
}
