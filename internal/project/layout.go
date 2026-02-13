package project

import (
	"context"
	"errors"
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
	RelLayoutPath string
	BaseDir       string
}

type LayoutCacheEntry struct {
	compiled CompiledLayout
	mtime    time.Time
	size     int64
	deps     []string
}

type LayoutRegistry interface {
	Get(ctx context.Context, key LayoutKey) (CompiledLayout, error)
	Invalidate(key LayoutKey)
	LayoutsForDir(ctx context.Context, path string, search bool) (map[string]CompiledLayout, error)
}

type CompiledLayout interface {
	ExecuteString(m map[string]interface{}) string
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

func (p *Project) templateForLayout(ctx context.Context, key LayoutKey) (*fasttemplate.Template, error) {
	log.WWarn(log.DEPRECATE).Msg("templateForLayout -> GetLayout")

	return nil, nil
}

func (lc *LayoutCache) Get(
	ctx context.Context,
	key LayoutKey,
) (CompiledLayout, error) {

	if tpl := lc.getCached(key); tpl != nil {
		return tpl, nil
	}

	log.DDebug(log.Layout).
		Str("rel_layout", key.RelLayoutPath).
		Str("base_dir", key.BaseDir).
		Msg("cache miss; building template")

	start := time.Now()
	v, err, _ := lc.sf.Do(key.RelLayoutPath+"|"+key.BaseDir, func() (any, error) {
		if tpl := lc.getCached(key); tpl != nil {
			return tpl, nil
		}

		tpl, err := lc.compile(ctx, key)
		if err != nil {
			return nil, err
		}

		log.DDebug(log.Layout).
			Str("rel_layout", key.RelLayoutPath).
			Dur("duration", time.Since(start)).
			Msg("template built and cached")

		return tpl, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*fasttemplate.Template), nil
}

func (lc *LayoutCache) compile(ctx context.Context, key LayoutKey) (CompiledLayout, error) {

	// TODO check: previously was key.RelLayoutPath used
	layoutPath := filepath.Join(key.BaseDir, key.RelLayoutPath)
	b, err := lc.fs.ReadFile(layoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read layout file %q: %w", layoutPath, err)
	}

	// TODO set correct value
	lyt, err := layout.Parse(key.RelLayoutPath, b /*p.Config.FsWatcher.HotReload*/, true)
	if err != nil {
		return nil, fmt.Errorf("failed to parse layout file %q: %w", layoutPath, err)
	}

	rctx := layout.RenderCtx{
		Pipelines: lc.pipelines,
		FS:        lc.fs,
	}
	str, deps, err := lyt.Render(ctx, rctx, key.BaseDir)
	if err != nil {
		return nil, err
	}

	tpl, err := fasttemplate.NewTemplate(str, "{{", "}}")
	if err != nil {
		return nil, err
	}

	fi, err := lc.fs.Stat(layoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat layout file %q: %w", layoutPath, err)
	}

	lc.mu.Lock()
	lc.data[key] = LayoutCacheEntry{
		compiled: tpl,
		deps:     deps,
		mtime:    fi.ModTime(),
		size:     fi.Size(),
	}
	lc.mu.Unlock()

	log.DDebug(log.Layout).
		Str("path", layoutPath).
		Int("contents", len(lyt.Contents())).
		Int("fragments", len(lyt.Fragments())).
		Msg("layout compiled")

	return tpl, nil
}

// GetLayouts returns all layouts for a given path
func (p *Project) GetLayouts(base string) (map[string]*layout.Layout, error) {
	log.WWarn(log.DEPRECATE).Msg("GetLayouts -> LayoutsForDir(true)")

	return nil, nil
}

// getLayoutsForDir checks if a specific directory has its own layout
func (p *Project) getLayoutsForDir(dir string) (map[string]*layout.Layout, error) {
	log.WWarn(log.DEPRECATE).Msg("getLayoutsForDir -> LayoutsForDir(false)")
	return nil, nil
}

func (lc *LayoutCache) LayoutsForDir(ctx context.Context, path string, search bool) (map[string]CompiledLayout, error) {
	matches, err := lc.fs.FindFile(path, lc.namePattern)
	if err != nil {
		return nil, fmt.Errorf("cannot find layout for path %q: %w", path, err)
	}

	out := make(map[string]CompiledLayout, len(matches))
	var errs []error
	for _, rel := range matches {
		relSlash := filepath.ToSlash(rel)

		lyt, err := lc.Get(ctx, LayoutKey{
			RelLayoutPath: relSlash,
			BaseDir:       path,
		})
		if err != nil {
			if search {
				errs = append(errs, fmt.Errorf("load layout %s: %w", relSlash, err))
				continue
			} else {
				return nil, fmt.Errorf("load layout %s: %w", relSlash, err)
			}
		}

		out[relSlash] = lyt

	}

	return out, errors.Join(errs...)
}

func (lc *LayoutCache) getCached(key LayoutKey) CompiledLayout {
	lc.mu.RLock()
	entry, exists := lc.data[key]
	lc.mu.RUnlock()

	if !exists {
		return nil
	}

	fi, err := lc.fs.Stat(filepath.Join(key.BaseDir, key.RelLayoutPath))
	if err != nil {
		return nil
	}

	if entry.size != fi.Size() && !entry.mtime.Equal(fi.ModTime()) {
		return nil
	}

	return entry.compiled
}

func (lc *LayoutCache) Invalidate(key LayoutKey) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	delete(lc.data, key)

	log.DDebug(log.Layout).
		Str("rel_layout", key.RelLayoutPath).
		Str("base_dir", key.BaseDir).
		Msg("layout key invalidated")
}
