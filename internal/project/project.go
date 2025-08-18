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

	log.IInfo(log.Pipeline).Str("pipelines", strings.Join(reg.Loaded(), ", ")).Msg("pipelines loaded")

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

	log.IInfo(log.Build).
		Msg("build started")

	plan, err := p.plan()
	// litter.Dump(plan)
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

		log.TTrace(log.Build).Int("tasks", len(assets)).Msg("assets batch run")
		err = executor.BatchExecutor{Concurrency: opt.Concurrency, BestEffort: true}.Run(ctx, assets)
		if err != nil {
			if unerr, ok := err.(interface{ Unwrap() []error }); ok {
				stats.Errors = append(stats.Errors, unerr.Unwrap()...)
			} else {
				log.EError(log.Build).Err(fmt.Errorf("assets batch executor: %w", err))
			}
		}
	} else {
		log.TTrace(log.Build).Msg("no assets to run")
	}

	for key, pages := range plan.PagesByLayout {
		if len(pages) == 0 {
			continue
		}
		stats.PagesPlanned += len(pages)

		log.IInfo(log.Build).
			Str("layout", key.BaseDir).
			Int("pages_planned", len(pages)).
			Msg("index phase")

		idxTasks := make([]executor.Task, 0, len(pages))
		for i := range pages {
			idxTasks = append(idxTasks, IndexTask{P: p, Page: pages[i]})
		}

		log.TTrace(log.Build).Int("tasks", len(idxTasks)).Str("layout", key.RelLayoutPath).Msg("index batch run")
		err = executor.BatchExecutor{Concurrency: opt.Concurrency, BestEffort: true}.Run(ctx, idxTasks)

		if err != nil {
			if unerr, ok := err.(interface{ Unwrap() []error }); ok {
				stats.Errors = append(stats.Errors, unerr.Unwrap()...)
			} else {
				log.EError(log.Build).Err(fmt.Errorf("index batch executor: %w", err))
			}
		}

		tpl, err := p.templateForLayout(ctx, key)
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

		log.TTrace(log.Build).Int("tasks", len(buildTasks)).Str("layout", key.RelLayoutPath).Msg("build batch run")
		err = executor.BatchExecutor{Concurrency: opt.Concurrency, BestEffort: true}.Run(ctx, buildTasks)
		if err != nil {
			if unerr, ok := err.(interface{ Unwrap() []error }); ok {
				stats.Errors = append(stats.Errors, unerr.Unwrap()...)
			} else {
				log.EError(log.Build).Err(fmt.Errorf("build batch executor: %w", err))
			}
		}
	}

	stats.Duration = time.Since(start)

	log.IInfo(log.Build).
		Dur("duration", stats.Duration).
		Int("assets_planned", stats.AssetsPlanned).
		Int64("assets_built", stats.AssetsBuilt).
		Int("pages_planned", stats.PagesPlanned).
		Int64("pages_built", stats.PagesBuilt).
		Int("errors", len(stats.Errors)).
		Msg("build finished")
	return stats, errors.Join(stats.Errors...)
}

func (p *Project) BuildPath(ctx context.Context, rel string) ([]string, error) {
	rel = filepath.ToSlash(rel)
	base := filepath.ToSlash(filepath.Dir(rel))
	filename := filepath.Base(rel)

	log.IInfo(log.Build).Str("rel", rel).Msg("building path")

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
		log.DDebug(log.Build).Str("rel", rel).Msg("no matching layouts")
		return nil, nil
	}

	body, err := p.readSource(rel)
	if err != nil {
		return nil, fmt.Errorf("read content %q: %w", rel, err)
	}

	var newPaths []string
	for _, m := range matches {
		tpl, err := p.templateForLayout(ctx, LayoutKey{RelLayoutPath: m.ldir, BaseDir: base})
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
			log.DDebug(log.Build).Str("dst", dst).Int("bytes", len(r.Result)).Msg("wrote file")
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

func (p *Project) templateForLayout(ctx context.Context, key LayoutKey) (*fasttemplate.Template, error) {
	lmap, err := p.getLayouts(key.BaseDir)
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
		Str("layout key base", key.BaseDir).
		Str("layout key rel", key.RelLayoutPath).
		Msg("template for layout")
	return p.layouts.GetOrBuild(ctx, p.schemeRoot, key, (*layout.Layout)(lyt))
}

// TODO: write file atomically:
// wait for go1.25: https://github.com/golang/go/issues/73041
func (p *Project) writeFile(root *os.Root, dst string, data []byte, perm fs.FileMode) error {
	dst = filepath.Clean(filepath.ToSlash(dst))

	log.TTrace(log.IO).Str("dst", dst).Int("bytes", len(data)).Msg("writeFile begin")

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

	log.TTrace(log.IO).Str("dst", dst).Uint32("perm", uint32(perm)).Msg("writeFile done")
	return f.Close()
}

func (p *Project) ignoreFile(name string) bool {
	name = filepath.ToSlash(filepath.Clean(name))

	log.TTrace(log.Project).Str("name", name).Msg("ignore check")

	for _, raw := range p.Config.Ignore {
		pat := filepath.ToSlash(strings.TrimSpace(raw))
		if pat == "" {
			continue
		}

		if strings.HasSuffix(pat, "/**") {
			prefix := strings.TrimSuffix(pat, "/**")
			if name == prefix || strings.HasPrefix(name, prefix+"/") {
				log.DDebug(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by prefix/**")
				return true
			}
			continue
		}
		if !strings.ContainsAny(pat, "*?[]") {
			if name == pat || strings.HasPrefix(name, pat+"/") {
				log.DDebug(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by exact/prefix")
				return true
			}
			continue
		}

		ok, err := path.Match(pat, name)
		if err != nil {
			continue
		}
		if ok {
			log.DDebug(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by glob")
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

	matches, err := utils.FindFile(p.schemeRoot.FS(), base, p.Config.Layout)
	if err != nil {
		return nil, fmt.Errorf("cannot find layout for path %q: %w", base, err)
	}

	log.DDebug(log.Layout).Str("base", base).Int("matches", len(matches)).Msg("layout matches")

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
			log.TTrace(log.Layout).Str("rel", relSlash).Msg("using cached layout")
			continue
		}

		b, err := fs.ReadFile(p.schemeRoot.FS(), relSlash)
		if err != nil {
			// return nil, fmt.Errorf("failed to read layout file %q: %w", layoutPath, err)
			continue
		}

		lyt, err := layout.Parse(p.schemeRoot, relSlash, b, p.pipelines, p.Config.FsWatcher.HotReload)
		if err != nil {
			// return nil, fmt.Errorf("failed to parse layout file %q: %w", layoutPath, err)
			continue
		}

		p.parsedMu.Lock()
		p.parsed[lyt.Path()] = parsedLayoutEntry{lyt: lyt, size: size, mtime: mtime}
		p.parsedMu.Unlock()

		log.TTrace(log.Layout).Str("rel", relSlash).Msg("parsed layout")
		out[relSlash] = lyt

	}

	return out, nil
}

func (p *Project) Close() error {
	var err error
	log.IInfo(log.Project).Msg("closing roots")
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
