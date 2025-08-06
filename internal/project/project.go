package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"craeft/internal/executor"
	"craeft/internal/layout"
	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/mattn/go-zglob"
	"github.com/sanity-io/litter"
	"github.com/valyala/fasttemplate"
)

var (
	blackList = []string{fmt.Sprintf("%s/**", projectRootFolder), projectRootFolder}
)

type Project struct {
	Server *Server
	Config *CraeftConfig

	srcRoot    *os.Root
	schemeRoot *os.Root
	outRoot    *os.Root

	closed atomic.Bool

	layouts *LayoutCache

	parsedOnce sync.Once
	parsedMu   sync.RWMutex
	parsed     map[string]parsedLayoutEntry
	pipelines  *pipeline.Registry
}

type BuildOptions struct {
	UseCache    bool
	Concurrency int
}

type BuildStats struct {
	PagesPlanned  int
	AssetsPlanned int
	PagesBuilt    int64
	AssetsBuilt   int64
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
		filepath.ToSlash(outputBase),
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
		PoolSize:        runtime.NumCPU(),
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
		_ = executor.BatchExecutor{Concurrency: opt.Concurrency, BestEffort: true}.Run(ctx, assets)
	}

	for key, pages := range plan.PagesByLayout {
		if len(pages) == 0 {
			continue
		}
		stats.PagesPlanned += len(pages)

		// TODO dont pass index vars to build
		sink := newIndexSink(len(pages))
		idxTasks := make([]executor.Task, 0, len(pages))
		for i := range pages {
			idxTasks = append(idxTasks, IndexTask{P: p, Page: pages[i], Sink: sink})
		}
		_ = executor.BatchExecutor{Concurrency: opt.Concurrency, BestEffort: true}.Run(ctx, idxTasks)

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
				Idx:     sink.get,
				Counter: &stats.PagesBuilt,
			})
		}
		_ = executor.BatchExecutor{Concurrency: opt.Concurrency, BestEffort: true}.Run(ctx, buildTasks)
	}

	fmt.Printf("Build took %s (pages %d/%d, assets %d/%d)\n",
		time.Since(start), stats.PagesBuilt, stats.PagesPlanned, stats.AssetsBuilt, stats.AssetsPlanned)
	return stats, nil
}

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
// // BuildPath builds a single page from a given path
// // The path should be relative to the project source
//
//	func (p *Project) BuildPath(path string) (string, error) {
//		base := filepath.Dir(path)
//		fileName := filepath.Base(path)
//
//		// TODO: check if the path is withing the project
//
//		layouts, err := p.getLayouts(base, false)
//		if err != nil {
//			return "", err
//		}
//
//		var outputPath string
//		for _, lyt := range layouts {
//			if lyt.MatchContent(fileName) {
//				for _, contentConf := range lyt.Contents() {
//					if contentConf.MatchContent(fileName) {
//
//						layoutTpl, err := lyt.Render(base)
//						if err != nil {
//							return "", err
//						}
//
//						// Index
//						page, err := p.getContent(contentConf.Name, path, contentConf.Pipe)
//						if err != nil {
//							return "", err
//						}
//
//						page.LoadIndex()
//
//						// Build
//						results, err := page.Build(layoutTpl)
//						if err != nil {
//							return "", err
//						}
//
//						outputPath, err = p.writeResults(base, results)
//						if err != nil {
//							return "", err
//						}
//
//					}
//				}
//
//			}
//
//		}
//
//		// For multiple files, it returns the last one
//		return outputPath, nil
//	}
func (p *Project) ignoreFile(path string) bool {
	path = filepath.Clean(path)
	for _, pat := range p.Config.Ignore {
		match, err := zglob.Match(pat, path)
		if err != nil {
			log.Error(err)
			return false
		}
		if match {
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

// func (p Project) writeResults(base string, results []ContentResult) (string, error) {
// 	var outputPath string
// 	targetDir := filepath.Join(p.Config.OutputPath, base)
// 	if err := os.MkdirAll(targetDir, 0755); err != nil {
// 		return "", err
// 	}
//
// 	for _, r := range results {
// 		outputPath = filepath.Join(targetDir, r.Filename)
// 		err := os.WriteFile(outputPath, []byte(r.Result), 0644)
// 		if err != nil {
// 			return "", err
// 		}
// 		log.Trace(fmt.Sprintf("content %s succesfuly written to: %s", r.Filename, outputPath))
// 	}
//
// 	return outputPath, nil
// }

//
// func (p *Project) getContent(name, path string, pipeline string) (*content.Content, error) {
// 	// var page *content.Content
// 	// if cached, ok := caches.contents.Load(path); ok {
// 	// 	page = cached.(*content.Content)
// 	// 	log.Infof("[del] Got page cache: %+v", page)
// 	// } else {
// 	pl, err := p.pipelines.Get(pipeline)
// 	if err != nil {
// 		if pipeline != "" {
// 			log.Warn(fmt.Sprintf("pipeline '%s' not found for content '%s': %s", pipeline, path, err))
// 		}
// 	}
// 	page, err := content.New(name, path, pl, p.Config.Variables)
// 	if err != nil {
// 		return nil, err
// 	}
// 	caches.contents.Store(path, page)
// 	// }
//
// 	return page, nil
// }
//
// type assetData struct {
// 	path     string
// 	from     string
// 	pipeline string
// }
//
// // TODO remove err return
// func (p *Project) getAsset(path string, pipeline string) (*content.Asset, error) {
// 	var asset *content.Asset
// 	// if cached, ok := caches.contents.Load(path); ok {
// 	// 	page = cached.(*content.Content)
// 	// 	log.Infof("[del] Got page cache: %+v", page)
// 	// } else {
// 	pl, err := p.pipelines.Get(pipeline)
// 	if err != nil {
// 		log.Warn(fmt.Sprintf("pipeline %s not found for asset %s: %w", pipeline, path, err))
// 	}
// 	asset = content.NewAsset(path, pl)
// 	// caches.contents.Store(path, page)
// 	// }
//
// 	return asset, nil
// }
//
// // walk walks the project source directory and calls the callback for each file
// var SkipFile = errors.New("skip this file")
//
// func (p *Project) walk(callback func(path string, d fs.DirEntry) error) error {
// 	return filepath.WalkDir(p.Config.SourcePath, func(path string, d fs.DirEntry, err error) error {
//
// 		rel, err := filepath.Rel(p.Config.SourcePath, path)
// 		if err != nil {
// 			return err
// 		}
//
// 		if p.ignoreFile(rel) {
// 			if d.IsDir() {
// 				return fs.SkipDir
// 			} else {
// 				return nil
// 			}
// 		}
//
// 		if err := callback(rel, d); err != nil {
// 			if err == SkipFile {
// 				return nil
// 			}
// 			return err
// 		}
//
// 		return nil
// 	})
// }

func (p Project) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}
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
