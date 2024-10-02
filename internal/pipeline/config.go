package pipeline

import (
	"os"

	"gopkg.in/yaml.v3"
)

type PipelineConfig struct {
	PipelineName string `yaml:"name"`
	EntryPoint   string `yaml:"entry_point"`
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

	return &config, nil
}
