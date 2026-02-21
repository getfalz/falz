package project

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"craeft/internal/layout"
	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/valyala/fasttemplate"
	"golang.org/x/sync/singleflight"
)

type LayoutKey struct {
	CtxPath    string // the path from where the layout was searched. Used to find fragments and contents relative to this path.
	LayoutPath string // the actual path to the layout file (incl. file name)
}

type LayoutCacheEntry struct {
	layoutTpl *fasttemplate.Template
	contents  map[string]*CompiledContent
	mtime     time.Time
	size      int64
	deps      []string
}

func (lc *LayoutCacheEntry) ExecuteString(vars map[string]interface{}) string {
	return lc.layoutTpl.ExecuteString(vars)
}

func (lc *LayoutCacheEntry) GetContent(name string) (*CompiledContent, bool) {
	c, ok := lc.contents[name]
	return c, ok
}

type LayoutRegistry interface {
	Get(ctx context.Context, key LayoutKey) (CompiledLayout, error)
	Parse(key LayoutKey) (*layout.Layout, error)
	Invalidate(key LayoutKey)
	LayoutsForDir(ctx context.Context, path string) (map[string]LayoutKey, error)
	LayoutsInDir(ctx context.Context, path string) (map[string]LayoutKey, error)
}

type CompiledLayout interface {
	ExecuteString(vars map[string]interface{}) string

	GetContent(name string) (*CompiledContent, bool)
}

type CompiledContent interface {
	OutputPath(entry *BuildEntry) (string, error)
}

type content struct {
	Name string
	OutputPathTpl *fasttemplate.Template
}

type LayoutCache struct {
	mu        sync.RWMutex
	data      map[LayoutKey]LayoutCacheEntry
	fs        *utils.RootFS
	sf        singleflight.Group
	pipelines *pipeline.Registry

	namePattern string
}

func NewLayoutCache(fs *utils.RootFS, pipes *pipeline.Registry, namePattern string) LayoutRegistry {
	log.DDebug(log.Layout).Msg("layout cache initialized")
	return &LayoutCache{
		data:        make(map[LayoutKey]LayoutCacheEntry),
		fs:          fs,
		pipelines:   pipes,
		namePattern: namePattern,
	}
}

func (lc *LayoutCache) Get(
	ctx context.Context,
	key LayoutKey,
) (CompiledLayout, error) {

	if tpl := lc.getCached(key); tpl != nil {
		return tpl, nil
	}

	log.DDebug(log.Layout).
		Str("ctx_path", key.CtxPath).
		Str("layout_path", key.LayoutPath).
		Msg("cache miss; building template")

	start := time.Now()
	v, err, _ := lc.sf.Do(key.CtxPath+"|"+key.LayoutPath, func() (any, error) {
		if tpl := lc.getCached(key); tpl != nil {
			return tpl, nil
		}

		tpl, err := lc.compile(ctx, key)
		if err != nil {
			return nil, err
		}

		log.DDebug(log.Layout).
			Str("rel_layout", key.CtxPath).
			Dur("duration", time.Since(start)).
			Msg("template built and cached")

		return tpl, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*fasttemplate.Template), nil
}

func (lc *LayoutCache) Parse(key LayoutKey) (*layout.Layout, error) {
	// TODO check: previously was key.RelLayoutPath used
	b, err := lc.fs.ReadFile(key.LayoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read layout file %q: %w", key.LayoutPath, err)
	}

	// TODO set correct value
	lyt, err := layout.Parse(key.CtxPath, b /*p.Config.FsWatcher.HotReload*/, true)
	if err != nil {
		return nil, fmt.Errorf("failed to parse layout file %q: %w", key.LayoutPath, err)
	}
	return lyt, nil
}

func (lc *LayoutCache) compile(ctx context.Context, key LayoutKey) (CompiledLayout, error) {
	lyt, err := lc.Parse(key)
	if err != nil {
		return nil, fmt.Errorf("failed to load layout: %w", err)
	}

	rctx := layout.RenderCtx{
		Pipelines: lc.pipelines,
		FS:        lc.fs,
	}
	str, deps, err := lyt.Render(ctx, rctx, key.CtxPath)
	if err != nil {
		return nil, err
	}

	tpl, err := fasttemplate.NewTemplate(str, "{{", "}}")
	if err != nil {
		return nil, err
	}

	fi, err := lc.fs.Stat(key.LayoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat layout file %q: %w", key.LayoutPath, err)
	}

	lc.mu.Lock()
	lc.data[key] = LayoutCacheEntry{
		layoutTpl: tpl,
		deps:      deps,
		mtime:     fi.ModTime(),
		size:      fi.Size(),
	}
	lc.mu.Unlock()

	log.DDebug(log.Layout).
		Str("path", key.LayoutPath).
		Int("contents", len(lyt.Contents())).
		Int("fragments", len(lyt.Fragments())).
		Msg("layout compiled")

	return tpl, nil
}

func (lc *LayoutCache) LayoutsInDir(ctx context.Context, path string) (map[string]LayoutKey, error) {
	glob := filepath.Join(path, lc.namePattern)
	glob = filepath.ToSlash(glob)
	matches, err := lc.fs.Glob(glob)

	if err != nil {
		return nil, fmt.Errorf("coudn't glob %q layouts: %w", glob, err)
	} else if len(matches) == 0 {
		return nil, fmt.Errorf("no layouts found in dir %s", path)
	}

	out := make(map[string]LayoutKey, len(matches))
	for _, layoutPath := range matches {

		out[layoutPath] = LayoutKey{
			CtxPath:    path,
			LayoutPath: layoutPath,
		}

	}

	return out, nil
}

func (lc *LayoutCache) LayoutsForDir(ctx context.Context, path string) (map[string]LayoutKey, error) {
	matches, err := lc.fs.FindFile(path, lc.namePattern)
	if err != nil {
		return nil, fmt.Errorf("cannot find layout for path %q: %w", path, err)
	}

	out := make(map[string]LayoutKey, len(matches))
	for _, layoutPath := range matches {

		out[layoutPath] = LayoutKey{
			CtxPath:    path,
			LayoutPath: layoutPath,
		}

	}

	return out, nil
}

func (lc *LayoutCache) getCached(key LayoutKey) CompiledLayout {
	lc.mu.RLock()
	entry, exists := lc.data[key]
	lc.mu.RUnlock()

	if !exists {
		return nil
	}

	fi, err := lc.fs.Stat(key.LayoutPath)
	if err != nil {
		return nil
	}

	if entry.size != fi.Size() && !entry.mtime.Equal(fi.ModTime()) {
		return nil
	}

	return entry.layoutTpl
}

func (lc *LayoutCache) Invalidate(key LayoutKey) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	delete(lc.data, key)

	log.DDebug(log.Layout).
		Str("rel_layout", key.CtxPath).
		Str("base_dir", key.LayoutPath).
		Msg("layout key invalidated")
}
