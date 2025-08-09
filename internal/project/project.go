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

	"craeft/internal/executor"
	"craeft/internal/layout"
	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/sanity-io/litter"
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
	Server *Server
	Config *CraeftConfig

	srcRoot    *os.Root
	schemeRoot *os.Root
	outRoot    *os.Root

	layouts *LayoutCache

	parsedOnce sync.Once
	parsedMu   sync.RWMutex
	parsed     map[string]parsedLayoutEntry
	pipelines  *pipeline.Registry

	lastPlanMu sync.RWMutex
	lastPlan   planSnapshot
}

type BuildOptions struct {
	UseCache    bool
	Concurrency int
}

type BuildStats struct {
	Duration time.Duration

	AssetsPlanned int
	AssetsBuilt   int64

	PagesPlanned int
	PagesBuilt   int64

	// TODO use a custom BuildError?
	Errors []error
}

func New(config *CraeftConfig) (*Project, error) {
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

	srcRoot, err := os.OpenRoot(srcAbs)
	if err != nil {
		return nil, fmt.Errorf("open root (source): %w", err)
	}
	schemeRoot, err := os.OpenRoot(schemeAbs)
	if err != nil {
		_ = srcRoot.Close()
		return nil, fmt.Errorf("open root (scheme): %w", err)
	}
	outRoot, err := os.OpenRoot(outAbs)
	if err != nil {
		_ = srcRoot.Close()
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
		_ = srcRoot.Close()
		_ = schemeRoot.Close()
		_ = outRoot.Close()
		return nil, err
	}

	return &Project{
		Config:     config,
		pipelines:  reg,
		layouts:    NewLayoutCache(),
		srcRoot:    srcRoot,
		schemeRoot: schemeRoot,
		outRoot:    outRoot,
	}, nil
}

func (p *Project) Build(ctx context.Context, opt BuildOptions) (BuildStats, error) {
	start := time.Now()
	stats := BuildStats{}

	plan, err := p.plan()
	litter.Dump(plan)
	if err != nil {
		fmt.Printf("Build failed in %s\n", time.Since(start))
		return stats, err
	}

	if n := len(plan.Assets); n > 0 {
		stats.AssetsPlanned = n
		assets := make([]executor.Task, 0, n)
		for _, a := range plan.Assets {
			assets = append(assets, AssetTask{P: p, Asset: a, Counter: &stats.AssetsBuilt})
		}
		err = executor.BatchExecutor{Concurrency: opt.Concurrency, BestEffort: true}.Run(ctx, assets)
		if err != nil {
			if unerr, ok := err.(interface{ Unwrap() []error }); ok {
				stats.Errors = append(stats.Errors, unerr.Unwrap()...)
			} else {
				log.Error(fmt.Errorf("assets batch executor: %w", err))
			}
		}
	}

	for key, pages := range plan.PagesByLayout {
		if len(pages) == 0 {
			continue
		}
		stats.PagesPlanned += len(pages)

		idxTasks := make([]executor.Task, 0, len(pages))
		for i := range pages {
			idxTasks = append(idxTasks, IndexTask{P: p, Page: pages[i]})
		}
		err = executor.BatchExecutor{Concurrency: opt.Concurrency, BestEffort: true}.Run(ctx, idxTasks)

		if err != nil {
			if unerr, ok := err.(interface{ Unwrap() []error }); ok {
				stats.Errors = append(stats.Errors, unerr.Unwrap()...)
			} else {
				log.Error(fmt.Errorf("index batch executor: %w", err))
			}
		}

		tpl, err := p.templateForLayout(key)
		if err != nil {
			continue
		}

		buildTasks := make([]executor.Task, 0, len(pages))
		for i := range pages {
			buildTasks = append(buildTasks, BuildTask{
				P:       p,
				Page:    pages[i],
				Tpl:     tpl,
				Counter: &stats.PagesBuilt,
			})
		}
		err = executor.BatchExecutor{Concurrency: opt.Concurrency, BestEffort: true}.Run(ctx, buildTasks)
		if err != nil {
			if unerr, ok := err.(interface{ Unwrap() []error }); ok {
				stats.Errors = append(stats.Errors, unerr.Unwrap()...)
			} else {
				log.Error(fmt.Errorf("build batch executor: %w", err))
			}
		}
	}

	stats.Duration = time.Since(start)

	return stats, errors.Join(stats.Errors...)
}

func (p *Project) BuildPath(ctx context.Context, rel string) ([]string, error) {
	rel = filepath.ToSlash(rel)
	base := filepath.ToSlash(filepath.Dir(rel))
	filename := filepath.Base(rel)

	lmap, err := p.getLayouts(base)
	if err != nil {
		return nil, err
	}

	type match struct{ ldir, name, pipe string }
	var matches []match

	for ldir, lyt := range lmap {
		if lyt.MatchContent(filename) {
			for _, c := range lyt.Contents() {
				if c.MatchContent(filename) {
					matches = append(matches, match{ldir: ldir, name: c.Name, pipe: c.Pipe})
				}
			}
		}
	}

	if len(matches) == 0 {
		return nil, nil
	}

	body, err := p.readSource(rel)
	if err != nil {
		return nil, fmt.Errorf("read content %q: %w", rel, err)
	}

	var newPaths []string
	for _, m := range matches {
		tpl, err := p.templateForLayout(LayoutKey{RelLayoutPath: m.ldir, BaseDir: base})
		if err != nil {
			return nil, err
		}

		pl, err := p.pipelines.Get(m.pipe)
		if err != nil {
			log.Warn(fmt.Sprintf("pipeline '%s' not found for %s: %v", m.pipe, rel, err))
		}
		cfg := pl.Config()
		target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(rel), "."+cfg.TargetSuffix)

		vars := pipeline.NewVars(target)
		vars.Set("vars", p.Config.Variables)

		if err := pl.Index(ctx, body, vars); err != nil {
			return nil, fmt.Errorf("index %s: %w", rel, err)
		}

		transformed, err := pl.Transform(ctx, body, vars)
		if err != nil {
			return nil, fmt.Errorf("transform %s: %w", rel, err)
		}
		if len(transformed) == 0 {
			continue
		}

		outs := make([]ContentResult, 0, len(transformed))
		for _, r := range transformed {
			if fn := r["filename"]; fn != "" {
				vars.Set(m.name, r["result"])
				out := tpl.ExecuteString(vars)
				outs = append(outs, ContentResult{Filename: fn, Result: out})
			}
		}
		dir := filepath.ToSlash(filepath.Join(base))

		for _, r := range outs {
			dst := filepath.ToSlash(filepath.Join(dir, r.Filename))
			if err := p.writeFile(p.outRoot, dst, []byte(r.Result), 0o644); err != nil {
				return nil, fmt.Errorf("write %s: %w", dst, err)
			}
		}

		newPaths = append(newPaths, filepath.Join(p.Config.OutputPath, filepath.FromSlash(rel)))
	}

	return newPaths, nil
}

func (p *Project) Pipelines() *pipeline.Registry { return p.pipelines }
func (p *Project) Layouts() *LayoutCache         { return p.layouts }

func (p *Project) readSource(rel string) ([]byte, error) {
	return fs.ReadFile(p.srcRoot.FS(), filepath.ToSlash(rel))
}

func (p *Project) readScheme(rel string) ([]byte, error) {
	return fs.ReadFile(p.schemeRoot.FS(), filepath.ToSlash(rel))
}

func (p *Project) templateForLayout(key LayoutKey) (*fasttemplate.Template, error) {
	lmap, err := p.getLayouts(key.BaseDir)
	if err != nil {
		return nil, err
	}
	lyt, ok := lmap[key.RelLayoutPath]
	if !ok || lyt == nil {
		return nil, fmt.Errorf("layout not found for %+v", key)
	}
	return p.layouts.GetOrBuild(p.schemeRoot, key, (*layout.Layout)(lyt))
}

// TODO: write file atomically:
// wait for go1.25: https://github.com/golang/go/issues/73041
func (p *Project) writeFile(root *os.Root, dst string, data []byte, perm fs.FileMode) error {
	dst = filepath.Clean(filepath.ToSlash(dst))

	if err := utils.MkdirAllFS(root, filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(dst), err)
	}

	f, err := root.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open %q: %w", dst, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write %q: %w", dst, err)
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return fmt.Errorf("chmod %q: %w", dst, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("fsync %q: %w", dst, err)
	}
	return f.Close()
}

func (p *Project) ignoreFile(name string) bool {
	name = filepath.ToSlash(filepath.Clean(name))

	for _, raw := range p.Config.Ignore {
		pat := filepath.ToSlash(strings.TrimSpace(raw))
		if pat == "" {
			continue
		}

		if strings.HasSuffix(pat, "/**") {
			prefix := strings.TrimSuffix(pat, "/**")
			if name == prefix || strings.HasPrefix(name, prefix+"/") {
				return true
			}
			continue
		}
		if !strings.ContainsAny(pat, "*?[]") {
			if name == pat || strings.HasPrefix(name, pat+"/") {
				return true
			}
			continue
		}

		ok, err := path.Match(pat, name)
		if err != nil {
			continue
		}
		if ok {
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
	p.parsedOnce.Do(func() {
		p.parsed = make(map[string]parsedLayoutEntry)
	})
}

// // GetLayout returns all layouts for a given path
func (p *Project) getLayouts(base string) (map[string]*layout.Layout, error) {
	p.ensureParsedCache()

	matches, err := utils.FindFile(p.Config.SchemePath, base, p.Config.Layout)
	if err != nil {
		return nil, fmt.Errorf("cannot find layout for path %q: %w", base, err)
	}

	out := make(map[string]*layout.Layout, len(matches))
	for _, rel := range matches {
		relSlash := filepath.ToSlash(rel)

		fi, err := p.schemeRoot.Stat(relSlash)
		if err != nil {
			continue
		}
		size, mtime := fi.Size(), fi.ModTime()

		p.parsedMu.RLock()
		e, ok := p.parsed[relSlash]
		p.parsedMu.RUnlock()

		if ok && e.size == size && e.mtime.Equal(mtime) {
			out[e.lyt.Path()] = e.lyt
			log.Trace(fmt.Sprintf("using cached layout %s", relSlash))
			continue
		}

		b, err := fs.ReadFile(p.schemeRoot.FS(), relSlash)
		if err != nil {
			// return nil, fmt.Errorf("failed to read layout file %q: %w", layoutPath, err)
			continue
		}

		lyt, err := layout.Parse(p.Config.SchemePath, relSlash, b, p.pipelines, p.Config.FsWatcher.HotReload)
		if err != nil {
			// return nil, fmt.Errorf("failed to parse layout file %q: %w", layoutPath, err)
			continue
		}

		p.parsedMu.Lock()
		p.parsed[relSlash] = parsedLayoutEntry{lyt: lyt, size: size, mtime: mtime}
		p.parsedMu.Unlock()

		log.Trace(fmt.Sprintf("using layout %s", relSlash))
		out[lyt.Path()] = lyt

	}

	return out, nil
}

func (p *Project) Close() error {
	var err error
	if p.srcRoot != nil {
		if e := p.srcRoot.Close(); e != nil {
			err = errors.Join(err, e)
		}
	}
	if p.schemeRoot != nil {
		if e := p.schemeRoot.Close(); e != nil {
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
