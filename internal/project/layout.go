package project

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"craeft/internal/layout"
	"craeft/internal/log"
	"craeft/internal/utils"

	"github.com/valyala/fasttemplate"
	"golang.org/x/sync/singleflight"
)

type LayoutKey struct {
	RelLayoutPath string
	BaseDir       string
}

type assetKey struct {
	From string
	Pipe string
	Rel  string
}

type LayoutCacheEntry struct {
	tpl  *fasttemplate.Template
	deps []string
}

type depIndex map[LayoutKey]struct{}

type LayoutCache struct {
	mu      sync.RWMutex
	data    map[LayoutKey]LayoutCacheEntry
	reverse map[string]depIndex
	sf      singleflight.Group
}

func NewLayoutCache() *LayoutCache {
	log.DDebug(log.Layout).Msg("layout cache initialized")
	return &LayoutCache{
		data:    make(map[LayoutKey]LayoutCacheEntry),
		reverse: make(map[string]depIndex),
	}
}

func (lc *LayoutCache) GetOrBuild(
	ctx context.Context,
	schemeRoot *utils.RootFS,
	key LayoutKey,
	lyt *layout.Layout,
) (*fasttemplate.Template, error) {

	if tpl := lc.get(key); tpl != nil {
		return tpl, nil
	}

	start := time.Now()
	log.DDebug(log.Layout).
		Str("rel_layout", key.RelLayoutPath).
		Str("base_dir", key.BaseDir).
		Int("fragments", len(lyt.Fragments())).
		Msg("cache miss; building template")

	v, err, _ := lc.sf.Do(key.RelLayoutPath+"|"+key.BaseDir, func() (any, error) {
		if tpl := lc.get(key); tpl != nil {
			return tpl, nil
		}

		str, deps, err := lyt.Render(ctx, key.BaseDir, schemeRoot)
		if err != nil {
			return nil, err
		}

		tpl, err := fasttemplate.NewTemplate(str, "{{", "}}")
		if err != nil {
			return nil, err
		}

		lc.mu.Lock()
		lc.data[key] = LayoutCacheEntry{
			tpl:  tpl,
			deps: deps,
		}

		for _, d := range deps {
			d = filepath.ToSlash(d)
			m := lc.reverse[d]
			if m == nil {
				m = depIndex{}
				lc.reverse[d] = m
			}
			m[key] = struct{}{}
		}
		lc.mu.Unlock()

		log.DDebug(log.Layout).
			Str("rel_layout", key.RelLayoutPath).
			Int("deps", len(deps)).
			Dur("duration", time.Since(start)).
			Msg("template built and cached")

		return tpl, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*fasttemplate.Template), nil
}

func (lc *LayoutCache) get(k LayoutKey) *fasttemplate.Template {
	lc.mu.RLock()
	e, ok := lc.data[k]
	lc.mu.RUnlock()
	if ok {
		return e.tpl
	}
	return nil
}

func (lc *LayoutCache) GetDeps(k LayoutKey) ([]string, bool) {
	lc.mu.RLock()
	e, ok := lc.data[k]
	lc.mu.RUnlock()
	if !ok {
		return nil, false
	}
	deps := make([]string, len(e.deps))
	copy(deps, e.deps)
	return deps, true
}

// (e.g., fragment) in the cache. It does not mutate state.
func (lc *LayoutCache) HasDep(path string) bool {
	p := filepath.ToSlash(path)
	lc.mu.RLock()
	_, ok := lc.reverse[p]
	lc.mu.RUnlock()
	return ok
}

func (lc *LayoutCache) InvalidateAndGetAffected(paths ...string) []LayoutKey {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	start := time.Now()

	keysToInvalidate := make(map[LayoutKey]struct{})
	var keysRemoved, reverseBucketsCleared int

	for _, p := range paths {
		p = filepath.ToSlash(p)
		for k := range lc.reverse[p] {
			keysToInvalidate[k] = struct{}{}
		}
	}

	depsToClean := make(map[string]struct{})
	for k := range keysToInvalidate {
		if entry, ok := lc.data[k]; ok {
			for _, dep := range entry.deps {
				depsToClean[dep] = struct{}{}
			}
			delete(lc.data, k)
			keysRemoved++
		}
	}

	for dep := range depsToClean {
		idx := lc.reverse[dep]
		for k := range keysToInvalidate {
			delete(idx, k)
		}
		if len(idx) == 0 {
			delete(lc.reverse, dep)
			reverseBucketsCleared++
		}
	}

	for _, p := range paths {
		delete(lc.reverse, filepath.ToSlash(p))
	}

	out := make([]LayoutKey, 0, len(keysToInvalidate))
	for k := range keysToInvalidate {
		out = append(out, k)
	}

	log.DDebug(log.Layout).
		Int("paths", len(paths)).
		Int("affected_keys", len(out)).
		Int("keys_removed", keysRemoved).
		Int("reverse_buckets_cleared", reverseBucketsCleared).
		Dur("duration", time.Since(start)).
		Msg("cache invalidated")

	return out
}

func (lc *LayoutCache) InvalidateByKey(key LayoutKey) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	entry, ok := lc.data[key]
	if !ok {
		return
	}

	// Remove from data
	delete(lc.data, key)

	// Clean up reverse index
	for _, dep := range entry.deps {
		if idx, ok := lc.reverse[dep]; ok {
			delete(idx, key)
			if len(idx) == 0 {
				delete(lc.reverse, dep)
			}
		}
	}

	log.DDebug(log.Layout).
		Str("rel_layout", key.RelLayoutPath).
		Str("base_dir", key.BaseDir).
		Int("deps", len(entry.deps)).
		Msg("layout key invalidated")
}
