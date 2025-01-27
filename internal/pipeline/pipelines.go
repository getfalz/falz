package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

type PipelineScript interface {
	Index(input []byte, vars Vars) Vars
	Transform(input []byte, vars Vars) []map[string]string
	Config() PipelineConfig
}

type Pipeline struct {
	*script
	config *PipelineConfig
}

// TODO rename file to pipeline.go
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

type RegistryConfig struct {
	Pipelines       []string
	PipelinesPath   string
	SourceDirectory string
}
type Registry struct {
	pipelines map[string]*Pipeline
	config    *RegistryConfig
	defaults  map[string]PipelineScript
}

func NewRegistry(config *RegistryConfig) *Registry {
	return &Registry{
		pipelines: make(map[string]*Pipeline),
		config:    config,
		defaults: map[string]PipelineScript{
			"tpl": &TplPipeline{},
			"md":  &MdPipeline{},
			"nop": &NopPipeline{},
		},
	}
}

func (r *Registry) LoadPipelines() error {
	for _, pipelineName := range r.config.Pipelines {
		pipelinePath := filepath.Join(r.config.PipelinesPath, pipelineName)

		if info, err := os.Stat(pipelinePath); os.IsNotExist(err) || !info.IsDir() {
			log.Warn("Pipeline directory does not exist or is not a directory: ", pipelinePath)
			continue
		}

		config := &JSConfig{
			Root: r.config.SourceDirectory,
		}
		pipeline, err := New(pipelinePath, config)
		if err != nil {
			return fmt.Errorf("error loading pipeline %s: %v", pipelineName, err)
		}

		log.Tracef("Loaded pipeline %s from %s", pipelineName, pipelinePath)
		r.pipelines[pipelineName] = pipeline
	}

	return nil
}

func (r *Registry) Get(name string) (PipelineScript, error) {
	if pipeline, ok := r.pipelines[name]; ok {
		return pipeline, nil
	}

	if defaultPipeline, ok := r.defaults[name]; ok {
		return defaultPipeline, nil
	}

	return r.defaults["nop"], fmt.Errorf("pipeline %s not found. using default", name)
}
