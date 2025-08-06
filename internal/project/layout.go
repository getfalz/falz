package project

import (
	"craeft/internal/layout"
	"os"
	"path/filepath"
	"sync"

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
    Rel  string // normalized, slash-separated, no leading "./"
}

type entry struct {
	tpl  *fasttemplate.Template
	deps []string
}

type depIndex map[LayoutKey]struct{} // small set

type LayoutCache struct {
	mu      sync.RWMutex
	data    map[LayoutKey]entry
	reverse map[string]depIndex
	sf      singleflight.Group
}

func NewLayoutCache() *LayoutCache {
	return &LayoutCache{
		data:    make(map[LayoutKey]entry),
		reverse: make(map[string]depIndex),
	}
}

// GetOrBuild compiles & stores layout.Render output.
func (lc *LayoutCache) GetOrBuild(
	schemeRoot *os.Root,
	key LayoutKey,
	lyt *layout.Layout,
) (*fasttemplate.Template, error) {

	if tpl := lc.get(key); tpl != nil {
		return tpl, nil
	}

	v, err, _ := lc.sf.Do(key.RelLayoutPath+"|"+key.BaseDir, func() (any, error) {
		if tpl := lc.get(key); tpl != nil {
			return tpl, nil
		}

		str, deps, err := lyt.RenderWithDeps(key.BaseDir, schemeRoot)
		if err != nil {
			return nil, err
		}
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

// InvalidatePaths is called by the watcher with changed scheme-relative paths.
func (lc *LayoutCache) InvalidatePaths(paths ...string) {
	lc.mu.Lock()
	for _, p := range paths {
		p = filepath.ToSlash(p)
		for k := range lc.reverse[p] {
			delete(lc.data, k)
			for _, e := range lc.reverse {
				delete(e, k)
			}
		}
		delete(lc.reverse, p)
	}
	lc.mu.Unlock()
}
