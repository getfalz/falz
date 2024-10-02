package pipeline

import "path/filepath"

type Pipeline struct {
	*script
	config *PipelineConfig
}

func New(path string) (*Pipeline, error) {

	configPath := filepath.Join(path, "config.yaml")
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	scriptPath := filepath.Join(path, config.EntryPoint)
	script, err := NewScript(scriptPath)
	if err != nil {
		return nil, err
	}

	return &Pipeline{
		script: script,
		config: config,
	}, nil
}
