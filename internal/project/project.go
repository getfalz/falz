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
	return p.layouts.GetOrBuild(ctx, p.schemeRoot, key, lyt)
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
