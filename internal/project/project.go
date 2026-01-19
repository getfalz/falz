package project

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
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

	SourceFS *utils.RootFS
	SchemeFS *utils.RootFS
	outputFS *utils.RootFS

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

	sourceRoot, err := utils.NewRootFs(config.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("source root fs: %w", err)
	}

	schemeRoot, err := utils.NewRootFs(config.SchemePath)
	if err != nil {
		_ = sourceRoot.Close()
		return nil, fmt.Errorf("scheme root fs: %w", err)
	}
	outputRoot, err := utils.NewRootFs(config.OutputPath)
	if err != nil {
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		return nil, fmt.Errorf("output root fs: %w", err)
	}

	outAbs, _ := filepath.Abs(config.OutputPath)
	srcAbs, _ := filepath.Abs(config.SourcePath)

	outputBase := filepath.Base(outAbs)
	config.Ignore = append(config.Ignore,
		filepath.ToSlash(filepath.Join(outputBase, "**")),
	)
	config.Ignore = append(config.Ignore, blackList...)

	reg := pipeline.NewRegistry(&pipeline.RegistryConfig{
		Pipelines:       config.Pipes,
		PipelinesPath:   config.PipesPath,
		PipeTimeout:     time.Duration(config.Build.PipeTimeout) * time.Second,
		SourceDirectory: srcAbs,
		PoolSize:        config.Build.Concurrency,
	})
	if err := reg.LoadAll(); err != nil {
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		_ = outputRoot.Close()
		return nil, err
	}

	log.IInfo(log.Pipeline).Str("pipelines", strings.Join(reg.Loaded(), ", ")).Msg("pipelines loaded")

	return &Project{
		Config:   config,
		Pipes:    reg,
		Layouts:  NewLayoutCache(),
		SourceFS: sourceRoot,
		SchemeFS: schemeRoot,
		outputFS: outputRoot,
	}, nil
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
	return p.Layouts.GetOrBuild(ctx, p.SchemeFS, key, lyt)
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

// GetLayouts returns all layouts for a given path
func (p *Project) GetLayouts(base string) (map[string]*layout.Layout, error) {
	p.ensureParsedCache()

	matches, err := p.SchemeFS.FindFile(base, p.Config.Layout)
	if err != nil {
		return nil, fmt.Errorf("cannot find layout for path %q: %w", base, err)
	}

	out := make(map[string]*layout.Layout, len(matches))
	var errs []error
	for _, rel := range matches {
		relSlash := filepath.ToSlash(rel)

		lyt, err := p.loadLayout(relSlash)
		if err != nil {
			errs = append(errs, fmt.Errorf("load layout %s: %w", relSlash, err))
			continue
		}

		out[relSlash] = lyt

	}

	return out, errors.Join(errs...)
}

func (p *Project) loadLayout(path string) (*layout.Layout, error) {

	fi, err := p.SchemeFS.Stat(path)
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

	b, err := p.SchemeFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read layout file %q: %w", path, err)
	}

	lyt, err := layout.Parse(p.SchemeFS, path, b, p.Pipes, p.Config.FsWatcher.HotReload)
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
	matches, err := p.SchemeFS.Glob(glob)

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
			return nil, err
		}

		out[relSlash] = lyt

	}

	return out, nil
}

func (p *Project) warmUpLayoutsByKeys(ctx context.Context, keys []LayoutKey) error {
	if len(keys) == 0 {
		return nil
	}
	var errs []error

	for _, k := range keys {
		lyt, err := p.loadLayout(k.RelLayoutPath)
		if err != nil || lyt == nil {
			errs = append(errs, fmt.Errorf("load layout %s: %w", k.RelLayoutPath, err))
			continue
		}

		if _, err := p.Layouts.GetOrBuild(ctx, p.SchemeFS, k, lyt); err != nil {
			errs = append(errs, fmt.Errorf("build layout %s: %w", k.RelLayoutPath, err))
			continue
		}
	}
	return errors.Join(errs...)
}
func (p *Project) InvalidateLayouts(ctx context.Context, plan *Plan, layoutKeys []LayoutKey, rescan bool) error {
	if len(layoutKeys) == 0 {
		return nil
	}

	var errs []error
	seenKey := make(map[LayoutKey]struct{}, len(layoutKeys))
	keys := make([]LayoutKey, 0, len(layoutKeys))

	for _, k := range layoutKeys {
		if _, ok := seenKey[k]; !ok {
			seenKey[k] = struct{}{}

			keys = append(keys, k)
			p.Layouts.InvalidateByKey(k)
		}
	}

	// if err := p.warmUpLayoutsByKeys(ctx, keys); err != nil {
	// 	return fmt.Errorf("warmup layouts: %w", err)
	// }

	if rescan {
		seenBase := make(map[string]struct{})
		for _, key := range keys {
			if _, ok := seenBase[key.BaseDir]; !ok {
				seenBase[key.BaseDir] = struct{}{}

				if _, err := p.Scan(ctx, plan, key.BaseDir, false); err != nil {
					errs = append(errs, fmt.Errorf("scan after layout invalidation for base %q: %w", key.BaseDir, err))
					continue
				}
			}
		}
		if _, err := p.HashDirty(ctx, plan); err != nil {
			return fmt.Errorf("hash dirty after layout invalidation: %w", err)
		}
	} else {
		plan.MarkDirtyByLayoutKeys(keys)
	}
	return errors.Join(errs...)
}

func (p *Project) CurrentPlan() (*Plan, error) {
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
	if e := p.SourceFS.Close(); e != nil {
		err = errors.Join(err, e)
	}
	if e := p.SchemeFS.Close(); e != nil {
		err = errors.Join(err, e)
	}
	if e := p.outputFS.Close(); e != nil {
		err = errors.Join(err, e)
	}
	return err
}
