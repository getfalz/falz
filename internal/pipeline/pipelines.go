package pipeline

import (
	"fmt"
	"path/filepath"
)

type Pipeline struct {
	*script
	config *PipelineConfig
}

func New(path string, jsconfig *JSConfig) (*Pipeline, error) {

	configPath := filepath.Join(path, "config.yaml")
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("[pipeline] %s", err)
	}

	scriptPath := filepath.Join(path, config.EntryPoint)
	script, err := NewScript(scriptPath, jsconfig)
	if err != nil {
		return nil, err
	}

	return &Pipeline{
		script: script,
		config: config,
	}, nil
}

func (p *Pipeline) Config() PipelineConfig {
	return *p.config
}
