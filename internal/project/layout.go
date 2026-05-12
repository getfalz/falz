package project

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
	layout    *layout.Layout
	layoutTpl *fasttemplate.Template

	// map of output path templates for the current layout
	outputs map[string]*fasttemplate.Template
	mtime   time.Time
	size    int64
	deps    []string
	tags    map[string]struct{} // a list of all tags used in the layout template
}

func (lc *LayoutCacheEntry) ExecuteString(vars map[string]interface{}) string {
	for k := range vars {
		if !(k == "target" || k == "vars") {
			if _, exists := lc.tags[k]; !exists {
				log.WWarn(log.Layout).Str("tag", k).Msg("variable provided but not found in template")
			}
		}
	}
	return lc.layoutTpl.ExecuteString(vars)
}

func (lc *LayoutCacheEntry) GetDeps() []string {
	return lc.deps
}

func (lc *LayoutCacheEntry) Layout() *layout.Layout {
	return lc.layout
}

var hasHashTag = regexp.MustCompile(`^HASH:(\d+)$`)

func (lc *LayoutCacheEntry) ContentOutput(contentName string, entry *BuildEntry) (string, error) {
	tpl, ok := lc.outputs[contentName]
	if !ok {
		return "", fmt.Errorf("content %q not found in layout", contentName)
	}
	if tpl == nil {
		return "", fmt.Errorf("content %q has no output template", contentName)
	}

	name := filepath.Base(entry.SourcePath)
	extWithDot := filepath.Ext(name)
	filename := strings.TrimSuffix(name, extWithDot)
	ext := strings.TrimPrefix(extWithDot, ".")

	// TODO don't panic on template parse
	// e.g. panic: unexpected error: unknown placeholder: FILENAM
	output := tpl.ExecuteFuncString(func(w io.Writer, tag string) (int, error) {
		switch tag {
		case "FILENAME":
			return w.Write([]byte(filename))
		case "EXT":
			return w.Write([]byte(ext))
		case "BASENAME":
			return w.Write([]byte(name))
		case "PIPE":
			return w.Write([]byte(entry.Pipeline))
		case "HASH":
			if entry.Hash == "" {
				return 0, fmt.Errorf("HASH not available yet")
			}
			return w.Write([]byte(entry.Hash))
		default:
			// HASH:N ?
			if m := hasHashTag.FindStringSubmatch(tag); len(m) == 2 {
				if entry.Hash == "" {
					return 0, fmt.Errorf("HASH not available yet")
				}

				n, err := strconv.Atoi(m[1])
				if err != nil || n <= 0 {
					return 0, fmt.Errorf("invalid HASH length: %q", m[1])
				}
				if n > len(entry.Hash) {
					n = len(entry.Hash)
				}
				return w.Write([]byte(entry.Hash[:n]))
			}

			return fmt.Fprintf(w, "[unknown tag %q]", tag)
		}
	})

	if output == "" || output == "." || output == ".." || strings.ContainsAny(output, "/\\") {
		return "", fmt.Errorf("invalid output path: %q", output)
	}

	return filepath.Join(entry.BaseDir, output), nil
}

type LayoutCache struct {
	mu        sync.RWMutex
	data      map[LayoutKey]*LayoutCacheEntry
	fs        *utils.RootFS
	sf        singleflight.Group
	pipelines *pipeline.Registry

	parsedMu sync.RWMutex
	parsed   map[string]*layout.Layout

	namePattern string
}

func NewLayoutCache(fs *utils.RootFS, pipes *pipeline.Registry, namePattern string) *LayoutCache {
	return &LayoutCache{
		data:        make(map[LayoutKey]*LayoutCacheEntry),
		parsed:      make(map[string]*layout.Layout),
		fs:          fs,
		pipelines:   pipes,
		namePattern: namePattern,
	}
}

func (lc *LayoutCache) Get(
	ctx context.Context,
	key LayoutKey,
) (*LayoutCacheEntry, error) {

	if tpl := lc.getCached(key); tpl != nil {
		return tpl, nil
	}

	cached, err, _ := lc.sf.Do(key.CtxPath+"|"+key.LayoutPath, func() (any, error) {
		if compiled := lc.getCached(key); compiled != nil {
			return compiled, nil
		}

		compiled, err := lc.compile(ctx, key)
		if err != nil {
			return nil, err
		}

		return compiled, nil
	})
	if err != nil {
		return nil, err
	}
	return cached.(*LayoutCacheEntry), nil
}

func (lc *LayoutCache) Parse(key LayoutKey) (*layout.Layout, error) {
	lc.parsedMu.RLock()
	if lyt, ok := lc.parsed[key.LayoutPath]; ok {
		lc.parsedMu.RUnlock()
		return lyt, nil
	}
	lc.parsedMu.RUnlock()

	// TODO check: previously was key.RelLayoutPath used
	b, err := lc.fs.ReadFile(key.LayoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read layout file %q: %w", key.LayoutPath, err)
	}

	lyt, err := layout.Parse(key.CtxPath, b)
	if err != nil {
		return nil, fmt.Errorf("failed to parse layout file %q: %w", key.LayoutPath, err)
	}

	lc.parsedMu.Lock()
	lc.parsed[key.LayoutPath] = lyt
	lc.parsedMu.Unlock()

	return lyt, nil
}

func (lc *LayoutCache) compile(ctx context.Context, key LayoutKey) (*LayoutCacheEntry, error) {
	start := time.Now()

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

	tags := make(map[string]struct{})
	tpl.ExecuteFunc(io.Discard, func(w io.Writer, tag string) (int, error) {
		tags[tag] = struct{}{}
		return 0, nil
	})

	outputs := make(map[string]*fasttemplate.Template, len(lyt.Contents()))
	for _, c := range lyt.Contents() {
		cToTpl, err := fasttemplate.NewTemplate(c.To, "{", "}")
		if err != nil {
			log.Warn(fmt.Sprintf("failed to parse content output template for content %q in layout %q: %v. This content will be ignored.", c.Name, key.LayoutPath, err))
			continue
		}
		outputs[c.Name] = cToTpl

		for _, asset := range c.Assets() {
			assetToTpl, err := fasttemplate.NewTemplate(asset.To, "{", "}")
			if err != nil {
				log.Warn(fmt.Sprintf("failed to parse content asset output template for asset %q in layout %q: %v. This asset will be ignored.", asset.Name, key.LayoutPath, err))
				continue
			}
			outputs[asset.Name] = assetToTpl
		}
	}
	for _, c := range lyt.Assets() {
		assetToTpl, err := fasttemplate.NewTemplate(c.To, "{", "}")
		if err != nil {
			log.Warn(fmt.Sprintf("failed to parse asset output template for asset %q in layout %q: %v. This asset will be ignored.", c.Name, key.LayoutPath, err))
			continue
		}
		outputs[c.Name] = assetToTpl
	}

	fi, err := lc.fs.Stat(key.LayoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat layout file %q: %w", key.LayoutPath, err)
	}

	entry := &LayoutCacheEntry{
		layout:    lyt,
		layoutTpl: tpl,
		tags:      tags,
		outputs:   outputs,
		deps:      deps,
		mtime:     fi.ModTime(),
		size:      fi.Size(),
	}
	lc.mu.Lock()
	lc.data[key] = entry
	lc.mu.Unlock()

	log.DDebug(log.Layout).
		Str("layout", key.LayoutPath).
		Str("ctx", key.CtxPath).
		Int("contents", len(lyt.Contents())).
		Int("fragments", len(lyt.Fragments())).
		Int("deps", len(deps)).
		Dur("duration", time.Since(start)).
		Msg("layout cached")

	return entry, nil
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

func (lc *LayoutCache) getCached(key LayoutKey) *LayoutCacheEntry {
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

	if entry.size != fi.Size() || !entry.mtime.Equal(fi.ModTime()) {
		return nil
	}

	return entry
}

func (lc *LayoutCache) Invalidate(key LayoutKey) {
	lc.mu.Lock()
	delete(lc.data, key)
	lc.mu.Unlock()

	lc.parsedMu.Lock()
	delete(lc.parsed, key.LayoutPath)
	lc.parsedMu.Unlock()

	log.DDebug(log.Layout).
		Str("rel_layout", key.CtxPath).
		Str("base_dir", key.LayoutPath).
		Msg("layout key invalidated")
}
