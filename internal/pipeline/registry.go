package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"craeft/internal/log"
	"craeft/internal/pipeline/js"
)

type RegistryConfig struct {
	Pipelines       []string
	PipelinesPath   string
	SourceDirectory string
	PoolSize        int
	PipeTimeout     time.Duration
}

type Registry struct {
	config   *RegistryConfig
	active   sync.Map
	defaults map[string]PipelineScript

	sfReload sync.Map
}

func NewRegistry(cfg *RegistryConfig) *Registry {
	return &Registry{
		config: cfg,
		defaults: map[string]PipelineScript{
			"tpl": &TplPipeline{},
			"md":  &MdPipeline{},
			"":    &NopPipeline{},
		},
	}
}

func (r *Registry) LoadAll() error {
	errs := []error{}
	var names []string
	for _, name := range r.config.Pipelines {
		if err := r.Reload(name); err != nil {
			log.Warn(fmt.Sprintf("load %s: %v", name, err))
			errs = append(errs, err)
		}
		names = append(names, name)
	}

	slices.Sort(names)
	log.IInfo(log.Pipeline).Str("pipelines", strings.Join(names, ", ")).Msg("pipelines loaded")

	return errors.Join(errs...)
}

func (r *Registry) Reload(name string) error {
	path := filepath.Join(r.config.PipelinesPath, name)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		if _, ok := r.defaults[name]; ok {
			return nil
		}
		return fmt.Errorf("pipeline dir invalid: %s", path)
	}

	jscfg := &js.JSConfig{
		Name:    name,
		Root:    path,
		Timeout: r.config.PipeTimeout,
	}

	newp, err := NewPipeline(path, jscfg, max(1, r.config.PoolSize))
	if err != nil {
		return err
	}

	old, _ := r.active.Load(name)
	r.active.Store(name, newp)

	if oldp, ok := old.(*Pipeline); ok && oldp != nil {
		go func(op *Pipeline) {
			// give in-flight users a grace period
			time.Sleep(100 * time.Millisecond)
			op.entry.close()
		}(oldp)
	}
	log.Trace(fmt.Sprintf("pipeline reloaded %s from %s", name, path))
	return nil
}

func (r *Registry) Get(name string) (PipelineScript, error) {
	if v, ok := r.active.Load(name); ok {
		return v.(*Pipeline), nil
	}
	if d, ok := r.defaults[name]; ok {
		return d, nil
	}
	// return r.defaults["nop"], fmt.Errorf("pipeline '%s' not found, using default", name)
	return nil, fmt.Errorf("pipeline '%s' not found", name)
}
