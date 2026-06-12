package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"craeft/internal/log"
	"craeft/internal/utils"
)

var ErrPipelineNotFound = errors.New("pipeline not found")

const SharedPipeFolder = "shared"

type Pipe interface {
	// Index(ctx context.Context, input []byte, vars Vars) error
	Transform(ctx context.Context, input []byte, vars Vars) ([]byte, error)
	Config() PipelineConfig
	Hash() string
}

type RegistryConfig struct {
	Pipes      []string
	PipesPath  string
	SourceRoot *utils.RootFS
}

// Registry maps pipe names to their Pipe implementation.
// Built-in pipes (md, tpl) are registered at construction.
// JS pipes are loaded from disk and can be reloaded at runtime.
type Registry struct {
	cfg        *RegistryConfig
	builtins   map[string]Pipe
	jsPipes    sync.Map // map[string]*JSPipe
	sharedRoot *utils.RootFS
}

func NewRegistry(cfg *RegistryConfig) (*Registry, error) {
	// sharedPath := filepath.Join(cfg.PipesPath, SharedPipeFolder)
	// TODO testing with root = pipesPath
	sharedRoot, err := utils.OpenRootFs(cfg.PipesPath)
	if err != nil {
		return nil, fmt.Errorf("shared root: %w", err)
	}
	return &Registry{
		cfg:        cfg,
		sharedRoot: sharedRoot,
		builtins: map[string]Pipe{
			"md":  &MdPipe{},
			"tpl": &TplPipe{},
			"":    &NopPipe{},
		},
	}, nil
}

func (r *Registry) LoadAll() error {
	var errs []error
	var names []string

	for _, name := range r.cfg.Pipes {
		if err := r.Load(name); err != nil {
			errs = append(errs, fmt.Errorf("pipe %q: %w", name, err))
			continue
		}
		names = append(names, name)
	}

	slices.Sort(names)
	log.IInfo(log.Pipeline).
		Str("pipes", strings.Join(names, ", ")).
		Msg("pipes loaded")

	return errors.Join(errs...)
}

// load creates a new JSPipe for the named pipe and stores it.
// If a pipe with that name already exists it is replaced and closed.
func (r *Registry) Load(name string) error {
	if name == SharedPipeFolder {
		log.WWarn(log.Pipeline).Msgf("pipe name %q is reserved; skipping", SharedPipeFolder)
		return nil
	}
	path := filepath.Join(r.cfg.PipesPath, name)
	info, err := os.Stat(path)

	if err != nil || !info.IsDir() {
		if _, isBuiltin := r.builtins[name]; isBuiltin {
			return nil
		}
		return fmt.Errorf("pipe %q: directory not found at %s", name, path)
	}

	pipe, err := NewJSPipe(path, r.cfg.SourceRoot, r.sharedRoot)
	if err != nil {
		return err
	}

	if old, ok := r.jsPipes.Swap(name, pipe); ok {
		old.(*JSPipe).Close()
	}

	return nil
}

// Reload rebuilds the JS bundle for a named pipe without replacing the JSPipe.
func (r *Registry) Reload(name string) error {
	v, ok := r.jsPipes.Load(name)
	if !ok {
		return fmt.Errorf("pipeline %q is not loaded (not activated in config)", name)
	}
	return v.(*JSPipe).Reload()
}

// Get returns the Pipe for the given name.
// Built-in pipes take precedence; JS pipes are looked up second.
func (r *Registry) Get(name string) (Pipe, error) {
	if v, ok := r.jsPipes.Load(name); ok {
		return v.(*JSPipe), nil
	}
	if p, ok := r.builtins[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("pipe %q: %w", name, ErrPipelineNotFound)

}

func (r *Registry) UnloadAll() {
	r.jsPipes.Range(func(key, value any) bool {
		value.(*JSPipe).Close()
		r.jsPipes.Delete(key)
		return true
	})
}

// Close releases all JS pipe resources (esbuild contexts).
func (r *Registry) Close() error {
	var errs []error
	r.jsPipes.Range(func(_, v any) bool {
		if err := v.(*JSPipe).Close(); err != nil {
			errs = append(errs, err)
		}
		return true
	})
	return errors.Join(errs...)
}
