package project

import (
	"craeft/internal/layout"
	"craeft/internal/log"
	"os"
	"path/filepath"
	"sync"
	"time"

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

type entry struct {
	tpl  *fasttemplate.Template
	deps []string
}

type depIndex map[LayoutKey]struct{}

type LayoutCache struct {
	mu      sync.RWMutex
	data    map[LayoutKey]entry
	reverse map[string]depIndex
	sf      singleflight.Group
}

func NewLayoutCache() *LayoutCache {
	log.DDebug(log.Layout).Msg("layout cache initialized")
	return &LayoutCache{
		data:    make(map[LayoutKey]entry),
		reverse: make(map[string]depIndex),
	}
}

func (lc *LayoutCache) GetOrBuild(
	schemeRoot *os.Root,
	key LayoutKey,
	lyt *layout.Layout,
) (*fasttemplate.Template, error) {

	if tpl := lc.get(key); tpl != nil {
		log.TTrace(log.Layout).
			Str("rel_layout", key.RelLayoutPath).
			Str("base_dir", key.BaseDir).
			Msg("cache hit")
		return tpl, nil
	}

	start := time.Now()
	log.DDebug(log.Layout).
		Str("rel_layout", key.RelLayoutPath).
		Str("base_dir", key.BaseDir).
		Msg("cache miss; building template")
	v, err, _ := lc.sf.Do(key.RelLayoutPath+"|"+key.BaseDir, func() (any, error) {
		if tpl := lc.get(key); tpl != nil {
			log.TTrace(log.Layout).
				Str("rel_layout", key.RelLayoutPath).
				Str("base_dir", key.BaseDir).
				Msg("singleflight: secondary hit after miss")
			return tpl, nil
		}

		str, deps, err := lyt.RenderWithDeps(key.BaseDir, schemeRoot)
		if err != nil {
			return nil, err
		}

		log.TTrace(log.Layout).
			Str("rel_layout", key.RelLayoutPath).
			Int("deps_count", len(deps)).
			Msg("rendered layout with deps")

		tpl, err := fasttemplate.NewTemplate(str, "{{", "}}")
		if err != nil {
			return nil, err
		}

		lc.mu.Lock()
		lc.data[key] = entry{tpl: tpl, deps: deps}
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
			Int("deps_count", len(deps)).
			Dur("build_ms", time.Since(start)).
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
		log.TTrace(log.Layout).
			Str("rel_layout", k.RelLayoutPath).
			Str("base_dir", k.BaseDir).
			Msg("get: hit")

		return e.tpl
	}
	return nil
}

func (lc *LayoutCache) InvalidateAndGetAffected(paths ...string) []LayoutKey {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	start := time.Now()

	seen := make(map[LayoutKey]struct{})
	var keysRemoved, reverseBucketsCleared int

	for _, p := range paths {
		p = filepath.ToSlash(p)
		keys := lc.reverse[p]
		for k := range keys {
			seen[k] = struct{}{}
			delete(lc.data, k)
			// remove k from every reverse bucket
			for dep, idx := range lc.reverse {
				delete(idx, k)
				if len(idx) == 0 {
					delete(lc.reverse, dep)
					reverseBucketsCleared++
				}
			}
			keysRemoved++
		}
		delete(lc.reverse, p)
	}
	out := make([]LayoutKey, 0, len(seen))
	for k := range seen {
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
