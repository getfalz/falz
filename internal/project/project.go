package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"craeft/internal/layout"
	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/valyala/fasttemplate"
)

var (
	blackList = []string{
		fmt.Sprintf("%s/**", projectRootFolder),
		"config.yaml",
		"*.dbg.*",
	}
)

type Project struct {
	Config *CraeftConfig

	SourceRoot *os.Root
	SchemeRoot *os.Root
	outRoot    *os.Root

	Layouts *LayoutCache

	parsedOnce sync.Once
	parsedMu   sync.RWMutex
	parsed     map[string]parsedLayoutEntry
	Pipes      *pipeline.Registry

	currentPlanMu sync.Mutex
	currentPlan   *Plan
}

func New(config *CraeftConfig) (*Project, error) {
	log.IInfo(log.Project).
		Str("src", config.SourcePath).
		Str("scheme", config.SchemePath).
		Str("out", config.OutputPath).
		Msg("project init")
	if err := config.IsValid(); err != nil {
		return nil, err
	}

	srcAbs, err := filepath.Abs(config.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("source abs: %w", err)
	}
	schemeAbs, err := filepath.Abs(config.SchemePath)
	if err != nil {
		return nil, fmt.Errorf("scheme abs: %w", err)
	}
	outAbs, err := filepath.Abs(config.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("output abs: %w", err)
	}
	if err := os.MkdirAll(outAbs, 0o755); err != nil {
		return nil, fmt.Errorf("ensure output: %w", err)
	}

	outputBase := filepath.Base(outAbs)
	config.Ignore = append(config.Ignore,
		filepath.ToSlash(filepath.Join(outputBase, "**")),
	)
	config.Ignore = append(config.Ignore, blackList...)

	sourceRoot, err := os.OpenRoot(srcAbs)
	if err != nil {
		return nil, fmt.Errorf("open root (source): %w", err)
	}
	schemeRoot, err := os.OpenRoot(schemeAbs)
	if err != nil {
		_ = sourceRoot.Close()
		return nil, fmt.Errorf("open root (scheme): %w", err)
	}
	outRoot, err := os.OpenRoot(outAbs)
	if err != nil {
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		return nil, fmt.Errorf("open root (output): %w", err)
	}

	reg := pipeline.NewRegistry(&pipeline.RegistryConfig{
		Pipelines:       config.Pipes,
		PipelinesPath:   config.PipesPath,
		SourceDirectory: srcAbs,
		// TODO set poolsize in config?
		PoolSize: runtime.NumCPU(),
	})
	if err := reg.LoadAll(); err != nil {
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		_ = outRoot.Close()
		return nil, err
	}

	log.IInfo(log.Pipeline).Str("pipelines", strings.Join(reg.Loaded(), ", ")).Msg("pipelines loaded")

	return &Project{
		Config:     config,
		Pipes:      reg,
		Layouts:    NewLayoutCache(),
		SourceRoot: sourceRoot,
		SchemeRoot: schemeRoot,
		outRoot:    outRoot,
	}, nil
}

func (p *Project) readSource(rel string) ([]byte, error) {
	return fs.ReadFile(p.SourceRoot.FS(), filepath.ToSlash(rel))
}

func (p *Project) readScheme(rel string) ([]byte, error) {
	return fs.ReadFile(p.SchemeRoot.FS(), filepath.ToSlash(rel))
}

func (p *Project) templateForLayout(ctx context.Context, key LayoutKey) (*fasttemplate.Template, error) {
	lmap, err := p.GetLayouts(key.BaseDir)
	if err != nil {
		return nil, err
	}
	keyPath := filepath.ToSlash(key.RelLayoutPath)
	lyt, ok := lmap[keyPath]

	if !ok || lyt == nil {
		return nil, fmt.Errorf("layout not found for %+v", key)
	}

	log.DDebug(log.Layout).
		Str("layout", lyt.Path()).
		Str("key_base", key.BaseDir).
		Str("key_rel", key.RelLayoutPath).
		Msg("template for layout")
	return p.Layouts.GetOrBuild(ctx, p.SchemeRoot, key, lyt)
}

func (p *Project) IgnoreFile(name string) bool {
	name = filepath.ToSlash(filepath.Clean(name))

	for _, raw := range p.Config.Ignore {
		pat := filepath.ToSlash(strings.TrimSpace(raw))
		if pat == "" {
			continue
		}

		if strings.HasSuffix(pat, "/**") {
			prefix := strings.TrimSuffix(pat, "/**")
			if name == prefix || strings.HasPrefix(name, prefix+"/") {
				log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by prefix/**")
				return true
			}
			continue
		}
		if !strings.ContainsAny(pat, "*?[]") {
			if name == pat || strings.HasPrefix(name, pat+"/") {
				log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by exact/prefix")
				return true
			}
			continue
		}

		ok, err := path.Match(pat, name)
		if err != nil {
			continue
		}
		if ok {
			log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by glob")
			return true
		}
	}
	return false
}

type parsedLayoutEntry struct {
	lyt   *layout.Layout
	size  int64
	mtime time.Time
}

func (p *Project) ensureParsedCache() {
	p.parsedMu.Lock()
	defer p.parsedMu.Unlock()
	p.parsedOnce.Do(func() {
		if p.parsed == nil {
			p.parsed = make(map[string]parsedLayoutEntry)
		}
	})
}

// // GetLayouts returns all layouts for a given path
func (p *Project) GetLayouts(base string) (map[string]*layout.Layout, error) {
	p.ensureParsedCache()

	matches, err := utils.FindFile(p.SchemeRoot.FS(), base, p.Config.Layout)
	if err != nil {
		return nil, fmt.Errorf("cannot find layout for path %q: %w", base, err)
	}

	out := make(map[string]*layout.Layout, len(matches))
	for _, rel := range matches {
		relSlash := filepath.ToSlash(rel)

		lyt, err := p.loadLayout(relSlash)
		if err != nil {
			continue
		}

		out[relSlash] = lyt

	}

	return out, nil
}

func (p *Project) loadLayout(path string) (*layout.Layout, error) {

	fi, err := p.SchemeRoot.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat layout file %q: %w", path, err)
	}
	size, mtime := fi.Size(), fi.ModTime()

	p.parsedMu.RLock()
	e, ok := p.parsed[path]
	p.parsedMu.RUnlock()

	if ok && e.size == size && e.mtime.Equal(mtime) {
		return e.lyt, nil
	}

	b, err := fs.ReadFile(p.SchemeRoot.FS(), path)
	if err != nil {
		return nil, fmt.Errorf("failed to read layout file %q: %w", path, err)
	}

	lyt, err := layout.Parse(p.SchemeRoot, path, b, p.Pipes, p.Config.FsWatcher.HotReload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse layout file %q: %w", path, err)
	}

	p.parsedMu.Lock()
	p.parsed[lyt.Path()] = parsedLayoutEntry{lyt: lyt, size: size, mtime: mtime}
	p.parsedMu.Unlock()

	log.DDebug(log.Layout).
		Str("path", path).
		Int("contents", len(lyt.Contents())).
		Int("fragments", len(lyt.Fragments())).
		Msg("layout loaded")

	return lyt, nil
}

// getLayoutsForDir checks if a specific directory has its own layout
func (p *Project) getLayoutsForDir(dir string) (map[string]*layout.Layout, error) {
	p.ensureParsedCache()

	glob := filepath.Join(dir, p.Config.Layout)
	glob = filepath.ToSlash(glob)
	matches, err := fs.Glob(p.SchemeRoot.FS(), glob)

	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", glob, err)
	} else if len(matches) == 0 {
		return nil, fmt.Errorf("layout no found in dir %s", dir)
	}

	out := make(map[string]*layout.Layout, len(matches))
	for _, rel := range matches {
		relSlash := filepath.ToSlash(rel)

		lyt, err := p.loadLayout(relSlash)
		if err != nil {
			log.WWarn(log.Layout).Err(err).Str("rel", relSlash).Msg("failed to load layout")
			continue
		}

		out[relSlash] = lyt

	}

	return out, nil
}

func (p *Project) CurrentPlan(ctx context.Context) (*Plan, error) {
	p.currentPlanMu.Lock()
	defer p.currentPlanMu.Unlock()

	if p.currentPlan == nil {
		return nil, fmt.Errorf("no current plan, call Plan() first")
	}
	return p.currentPlan, nil
}

func (p *Project) Close() error {
	var err error
	log.IInfo(log.Project).Msg("closing roots")
	if p.SourceRoot != nil {
		if e := p.SourceRoot.Close(); e != nil {
			err = errors.Join(err, e)
		}
	}
	if p.SchemeRoot != nil {
		if e := p.SchemeRoot.Close(); e != nil {
			err = errors.Join(err, e)
		}
	}
	if p.outRoot != nil {
		if e := p.outRoot.Close(); e != nil {
			err = errors.Join(err, e)
		}
	}
	return err
}
