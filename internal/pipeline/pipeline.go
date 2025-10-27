package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"craeft/internal/pipeline/js"
)

type PipelineScript interface {
	Index(ctx context.Context, input []byte, vars Vars) error
	Transform(ctx context.Context, input []byte, vars Vars) (string, error)
	Config() PipelineConfig
}

type envPool struct {
	name    string
	path    string
	code    string
	cfg     *js.JSConfig
	ch      chan *script
	closing atomic.Bool
	size    int
}

func newEnvPool(name, path, code string, cfg *js.JSConfig, n int) (*envPool, error) {
	p := &envPool{
		name: name,
		path: path,
		code: code,
		cfg:  cfg,
		ch:   make(chan *script, n),
		size: n,
	}
	for i := 0; i < n; i++ {
		w, err := NewScript(name, path, cfg, code)
		if err != nil {
			for {
				select {
				case s := <-p.ch:
					_ = s.env.Close()
				default:
					return nil, err
				}
			}
		}
		p.ch <- w
	}
	return p, nil
}

func (p *envPool) acquire(ctx context.Context) (*script, error) {
	select {
	case w := <-p.ch:
		return w, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *envPool) release(w *script) {
	if p.closing.Load() {
		_ = w.env.Close()
		return
	}
	select {
	case p.ch <- w:
	default:
		// pool full (shouldn't happen) — drop
		_ = w.env.Close()
	}
}

func (p *envPool) close() {
	if !p.closing.CompareAndSwap(false, true) {
		return
	}
loop:
	for {
		select {
		case w := <-p.ch:
			_ = w.env.Close()
		default:
			break loop
		}
	}
	close(p.ch)
}

type Pipeline struct {
	entry  *envPool
	config *PipelineConfig
}

func NewPipeline(path string, jsconfig *js.JSConfig, poolSize int) (*Pipeline, error) {
	configPath := filepath.Join(path, "config.yaml")
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("can't load pipeline config %s", err)
	}

	scriptPath := filepath.Join(path, config.EntryPoint)
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("entry not found: %w", err)
	}

	code, err := loadScript(scriptPath, filepath.Base(scriptPath), config.Debug)
	if err != nil {
		return nil, err
	}
	pool, err := newEnvPool(jsconfig.Name, scriptPath, code, jsconfig, poolSize)
	if err != nil {
		return nil, err
	}

	return &Pipeline{entry: pool, config: config}, nil
}

func (p *Pipeline) Config() PipelineConfig {
	return *p.config
}

func (p *Pipeline) Index(ctx context.Context, input []byte, vars Vars) error {
	w, err := p.entry.acquire(ctx)
	if err != nil {
		return err
	}
	defer p.entry.release(w)
	return w.Index(ctx, input, vars)
}

func (p *Pipeline) Transform(ctx context.Context, input []byte, vars Vars) (string, error) {
	w, err := p.entry.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer p.entry.release(w)
	return w.Transform(ctx, input, vars)
}
