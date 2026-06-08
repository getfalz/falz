package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"craeft/internal/pipeline/js"
	"craeft/internal/utils"

	"github.com/dop251/goja"
)

type JSPipe struct {
	cfg      vmConfig
	config   PipelineConfig
	bundle   *bundle
	pipeRoot *utils.RootFS

	prg        *goja.Program
	bundleHash string
	bundleMu   sync.RWMutex
}

func NewJSPipe(path string, srcRoot *utils.RootFS, sharedRoot *utils.RootFS,) (*JSPipe, error) {
	pcfg, err := loadConfig(filepath.Join(path, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	pipeRoot, err := utils.OpenRootFs(path)
	if err != nil {
		return nil, fmt.Errorf("pipe rootfs: %w", err)
	}

	b, err := newBundle(pcfg.EntryPoint, pipeRoot, sharedRoot, pcfg.Debug)
	if err != nil {
		_ = pipeRoot.Close()
		return nil, fmt.Errorf("new bundle: %w", err)
	}

	src, err := b.build(pcfg.Debug)
	if err != nil {
		b.close()
		_ = pipeRoot.Close()
		return nil, fmt.Errorf("initial build: %w", err)
	}

	prg, err := compileBundle(pcfg.PipelineName, src)
	if err != nil {
		b.close()
		_ = pipeRoot.Close()
		return nil, fmt.Errorf("compile: %w", err)
	}

	jsCfg := vmConfig{
		name:   pcfg.PipelineName,
		root:   srcRoot,
		dbPath: "",

		httpPolicy:    pcfg.Fetch.Policy(),
		httpTransport: js.NewSharedTransport(pcfg.Fetch.MaxConnsPerHost),
	}

	return &JSPipe{
		cfg:        jsCfg,
		config:     *pcfg,
		bundle:     b,
		pipeRoot:   pipeRoot,
		prg:        prg,
		bundleHash: utils.HashString(src),
	}, nil
}

// Reload rebuilds the bundle incrementally and swaps the bundle string.
// In-flight Transform calls finish with the old bundle; new calls pick
// up the new one. No VM draining required.
func (p *JSPipe) Reload() error {
	src, err := p.bundle.build(p.config.Debug)
	if err != nil {
		return err
	}

	prg, err := compileBundle(p.config.PipelineName, src)
	if err != nil {
		return fmt.Errorf("recompile: %w", err)
	}
	hash := utils.HashString(src)

	p.bundleMu.Lock()
	p.prg = prg
	p.bundleHash = hash
	p.bundleMu.Unlock()
	return nil
}

func (p *JSPipe) Config() PipelineConfig {
	return p.config
}

func (p *JSPipe) Hash() string {
	p.bundleMu.RLock()
	defer p.bundleMu.RUnlock()
	return p.bundleHash
}

func (p *JSPipe) Transform(ctx context.Context, input []byte, vars Vars) ([]byte, error) {
	p.bundleMu.RLock()
	prg := p.prg
	p.bundleMu.RUnlock()

	vm, err := newVMWorker(ctx, p.cfg, prg)
	if err != nil {
		return nil, fmt.Errorf("create vm: %w", err)
	}
	defer vm.close()

	return vm.transform(ctx, input, vars)
}

func (p *JSPipe) Close() error {
	p.bundle.close()
	return p.pipeRoot.Close()
}
