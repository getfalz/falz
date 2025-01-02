package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

type PipelineScript interface {
	Index(input string, vars map[string]string) map[string]string
	Transform(input string, vars map[string]string) []map[string]string
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
}

func NewRegistry(config *RegistryConfig) *Registry {
	return &Registry{
		pipelines: make(map[string]*Pipeline),
		config:    config,
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

type DefaultPipeline struct {
}

func (d DefaultPipeline) Config() PipelineConfig {
	return PipelineConfig{
		PipelineName: "default",
		EntryPoint:   "",
		SourceSuffix: "",
		TargetSuffix: "",
	}
}
func (d DefaultPipeline) Index(input string, vars map[string]string) map[string]string {
	return vars
}
func (d DefaultPipeline) Transform(input string, vars map[string]string) []map[string]string {
	results := []map[string]string{
		{
			"filename": vars["target"],
			"result":   input,
		},
	}
	return results
}

func (r *Registry) Get(name string, useDefault bool) (PipelineScript, error) {
	pipeline, ok := r.pipelines[name]

	if !ok {
		if useDefault {
			return &DefaultPipeline{}, nil
		}
		return nil, fmt.Errorf("Pipeline not found: %s", name)
	}
	return pipeline, nil
}
