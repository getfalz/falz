package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/project"
)

type Handlers struct {
	project *project.Project
	source  *project.SourceIndex
	hasher  *project.HashIndex
}

func NewHandlers(p *project.Project) *Handlers {
	return &Handlers{
		project: p,
		source:  project.NewSourceIndex(p.Source, p.Scheme, p.Layouts, p.Ignore),
		hasher:  project.NewHashIndex(p.Source, p.Scheme, p.Layouts, p.PipesRegistry),
	}
}

func (h *Handlers) RegisterAll(r *EventRegistry) {
	// Content handlers
	r.Register(FileTypeContent, ModTypeModified, HandlerFunc(h.handleContentModified))
	r.Register(FileTypeContent, ModTypeCreated, HandlerFunc(h.handleContentCreated))
	r.Register(FileTypeContent, ModTypeDeleted, HandlerFunc(h.handleContentDeleted))

	// Content Asset handlers
	r.Register(FileTypeContentAsset, ModTypeModified, HandlerFunc(h.handleContentAssetModified))
	r.Register(FileTypeContentAsset, ModTypeCreated, HandlerFunc(h.handleContentAssetCreated))
	r.Register(FileTypeContentAsset, ModTypeDeleted, HandlerFunc(h.handleContentAssetDeleted))

	// Layout handlers
	r.Register(FileTypeLayout, ModTypeModified, HandlerFunc(h.handleLayoutModified))
	r.Register(FileTypeLayout, ModTypeCreated, HandlerFunc(h.handleLayoutCreated))
	r.Register(FileTypeLayout, ModTypeDeleted, HandlerFunc(h.handleLayoutDeleted))

	// Fragment handlers
	r.Register(FileTypeFragment, ModTypeModified, HandlerFunc(h.handleFragmentModified))
	r.Register(FileTypeFragment, ModTypeCreated, HandlerFunc(h.handleFragmentModified))
	r.Register(FileTypeFragment, ModTypeDeleted, HandlerFunc(h.handleFragmentModified))

	// Scheme Asset handlers
	r.Register(FileTypeSchemeAsset, ModTypeModified, HandlerFunc(h.handleSchemeAssetModified))
	r.Register(FileTypeSchemeAsset, ModTypeCreated, HandlerFunc(h.handleSchemeAssetCreated))
	r.Register(FileTypeSchemeAsset, ModTypeDeleted, HandlerFunc(h.handleSchemeAssetDeleted))

	// Pipeline handlers
	r.Register(FileTypePipeline, ModTypeModified, HandlerFunc(h.handlePipelineModified))
	r.Register(FileTypePipeline, ModTypeCreated, HandlerFunc(h.handlePipelineModified))
	r.Register(FileTypePipeline, ModTypeDeleted, HandlerFunc(h.handlePipelineModified))

	r.Register(FileTypeConfig, ModTypeModified, HandlerFunc(h.handleConfigModified))
}

func (h *Handlers) handleContentModified(ctx context.Context, paths []string) error {
	for _, path := range paths {
		for _, entry := range h.project.Graph().EntriesBySource(project.SourceTypeSource, project.ContentType, path) {
			h.rehashAndMark(ctx, entry.ID)
		}
	}
	return nil
}

func (h *Handlers) handleContentCreated(ctx context.Context, paths []string) error {
	graph := h.project.Graph()

	for _, path := range paths {
		if existing := h.project.Graph().EntriesBySource(project.SourceTypeSource, project.ContentType, path); len(existing) > 0 {
			for _, e := range existing {
				h.rehashAndMark(ctx, e.ID)
			}
			continue
		}

		entries, ok := h.source.AddFile(ctx, graph, path, project.SourceTypeSource)
		if !ok {
			log.DDebug(log.Handler).Str("path", path).Msg("created file matched no content pattern")
			continue
		}

		for _, entry := range entries {
			if err := h.computeAndMarkDirty(ctx, entry); err != nil {
				log.WWarn(log.Handler).Err(err).Str("path", path).Msg("failed to hash new content")
				continue
			}
			log.IInfo(log.Handler).Str("id", entry.ID).Msg("new content added to graph")
		}
	}
	return nil
}

func (h *Handlers) handleContentDeleted(ctx context.Context, paths []string) error {
	for _, path := range paths {
		for _, entry := range h.project.Graph().EntriesBySource(project.SourceTypeSource, project.ContentType, path) {
			h.removeEntry(entry.ID)
		}
	}
	return nil
}

// --- Content Assets ---

func (h *Handlers) handleContentAssetModified(ctx context.Context, paths []string) error {
	for _, path := range paths {
		for _, entry := range h.project.Graph().EntriesBySource(project.SourceTypeSource, project.AssetType, path) {
			h.rehashAndMark(ctx, entry.ID)
		}
	}
	return nil
}

func (h *Handlers) handleContentAssetCreated(ctx context.Context, paths []string) error {
	graph := h.project.Graph()

	for _, path := range paths {
		found := false
		for _, entry := range graph.EntriesBySource(project.SourceTypeSource, project.AssetType, path) {
			found = true
			h.rehashAndMark(ctx, entry.ID)
		}
		if found {
			continue
		}

		entries, ok := h.source.AddFile(ctx, graph, path, project.SourceTypeSource)
		if !ok {
			log.DDebug(log.Handler).Str("path", path).Msg("created file matched no asset pattern")
			continue
		}

		for _, entry := range entries {
			if err := h.computeAndMarkDirty(ctx, entry); err != nil {
				log.WWarn(log.Handler).Err(err).Str("path", path).Msg("failed to hash new content")
				continue
			}
			log.IInfo(log.Handler).Str("id", entry.ID).Msg("new content asset added to graph")
		}

	}
	return nil
}

func (h *Handlers) handleContentAssetDeleted(ctx context.Context, paths []string) error {
	for _, path := range paths {
		for _, entry := range h.project.Graph().EntriesBySource(project.SourceTypeSource, project.AssetType, path) {
			h.removeEntry(entry.ID)
		}
	}
	return nil
}

// --- Layouts ---

func (h *Handlers) handleLayoutModified(ctx context.Context, paths []string) error {
	for _, path := range paths {
		log.IInfo(log.Handler).Str("layout", path).Msg("layout modified")
		h.invalidateAndRehash(ctx, path, false)
	}
	return nil
}

func (h *Handlers) handleLayoutCreated(ctx context.Context, paths []string) error {
	graph := h.project.Graph()

	for _, path := range paths {
		dir := filepath.Dir(path)
		log.IInfo(log.Handler).Str("layout", path).Str("dir", dir).Msg("layout created, rescanning directory")

		// Entries in this dir were bound to a parent layout; remove them so
		// PartialScan re-adds them under the new, more specific layout.
		for _, entry := range graph.AllEntries() {
			if entry.Type == project.ContentType && strings.HasPrefix(entry.BaseDir, dir) {
				graph.RemoveNode(project.NewEntryNode(entry))
			}
		}

		if err := h.source.PartialScan(ctx, graph, dir); err != nil {
			log.EError(log.Handler).Err(err).Str("dir", dir).Msg("partial scan failed")
			continue
		}

		h.hashNewEntries(ctx)
	}
	return nil
}

func (h *Handlers) handleLayoutDeleted(ctx context.Context, paths []string) error {
	graph := h.project.Graph()

	for _, path := range paths {
		dir := filepath.Dir(path)
		log.IInfo(log.Handler).Str("layout", path).Str("dir", dir).Msg("layout deleted, rescanning with parent layout")

		lkey := project.LayoutKey{CtxPath: dir, LayoutPath: path}
		layoutNode := project.NewLayoutNode(lkey)

		// Remove all entries that depended on this layout before rescanning.
		for _, n := range graph.AffectedBy(layoutNode) {
			if n.Kind() == project.NodeKindEntry {
				graph.RemoveNode(n)
			}
		}
		graph.RemoveNode(layoutNode)
		h.project.Layouts.Invalidate(lkey)

		if err := h.source.PartialScan(ctx, graph, dir); err != nil {
			log.EError(log.Handler).Err(err).Str("dir", dir).Msg("partial scan failed")
			continue
		}

		h.hashNewEntries(ctx)
	}
	return nil
}

// --- Fragments ---

// A fragment change never restructures the graph — it only invalidates the
// compiled layout cache and triggers a rehash of all dependent entries.
// Create/modify/delete are all treated the same way.
func (h *Handlers) handleFragmentModified(ctx context.Context, paths []string) error {
	for _, path := range paths {
		log.DDebug(log.Handler).Str("fragment", path).Msg("fragment changed")
		h.invalidateAndRehash(ctx, path, true)
	}
	return nil
}

// --- Scheme Assets ---

func (h *Handlers) handleSchemeAssetModified(ctx context.Context, paths []string) error {
	for _, path := range paths {
		for _, entry := range h.project.Graph().EntriesBySource(project.SourceTypeScheme, project.AssetType, path) {
			h.rehashAndMark(ctx, entry.ID)
		}
	}
	return nil
}

func (h *Handlers) handleSchemeAssetCreated(ctx context.Context, paths []string) error {
	graph := h.project.Graph()

	for _, path := range paths {
		found := false
		for _, entry := range graph.EntriesBySource(project.SourceTypeScheme, project.AssetType, path) {
			found = true
			h.rehashAndMark(ctx, entry.ID)
		}
		if found {
			continue
		}

		entries, ok := h.source.AddFile(ctx, graph, path, project.SourceTypeScheme)
		if !ok {
			log.DDebug(log.Handler).Str("path", path).Msg("created file matched no scheme asset pattern")
			continue
		}

		for _, entry := range entries {
			if err := h.computeAndMarkDirty(ctx, entry); err != nil {
				log.WWarn(log.Handler).Err(err).Str("path", path).Msg("failed to hash new content")
				continue
			}
			log.IInfo(log.Handler).Str("id", entry.ID).Msg("new scheme asset added to graph")
		}

	}
	return nil
}

func (h *Handlers) handleSchemeAssetDeleted(ctx context.Context, paths []string) error {
	for _, path := range paths {
		for _, entry := range h.project.Graph().EntriesBySource(project.SourceTypeScheme, project.AssetType, path) {
			h.removeEntry(entry.ID)
		}
	}
	return nil
}

// --- Pipelines ---

func (h *Handlers) handlePipelineModified(ctx context.Context, paths []string) error {
	graph := h.project.Graph()

	reloadAll := false
	pipeNames := make(map[string]struct{})
	for _, path := range paths {
		name, _, _ := strings.Cut(path, "/")
		if name == pipeline.SharedPipeFolder {
			reloadAll = true
		} else {
			pipeNames[name] = struct{}{}
		}
	}

	if reloadAll {
		for _, name := range h.project.Config.Pipes {
			pipeNames[name] = struct{}{}
		}
	}

	for pipeName := range pipeNames {
		if err := h.project.PipesRegistry.Reload(pipeName); err != nil {
			log.WWarn(log.Handler).Err(err).Str("pipeline", pipeName).Msg("pipeline reload failed")
			continue
		}
		log.IInfo(log.Handler).Str("pipeline", pipeName).Msg("pipeline reloaded")

		pipeNode, ok := graph.GetNode(fmt.Sprintf("pipeline:%s", pipeName))
		if !ok {
			continue
		}

		for _, n := range graph.DependentsOf(pipeNode) {
			switch dep := n.(type) {
			case *project.EntryNode:
				h.rehashAndMark(ctx, dep.Entry().ID)
			case *project.FragmentNode:
				h.invalidateAndRehash(ctx, dep.Path(), true)
			}
		}
	}
	return nil
}

// rehashAndMark recomputes the hash for an existing entry by ID and marks it
// dirty if the hash changed. No-ops silently if the entry is not in the graph
// or the hash is unchanged.
func (h *Handlers) rehashAndMark(ctx context.Context, id string) {
	graph := h.project.Graph()

	entry, ok := graph.EntryByID(id)
	if !ok {
		log.DDebug(log.Handler).
			Str("id", id).
			Msg("entry not in graph, skipping rehash — possibly a new file or editor temp")
		return
	}

	newHash, err := h.hasher.ComputeHash(ctx, entry)
	if err != nil {
		log.WWarn(log.Handler).Err(err).Str("id", id).Msg("failed to compute hash")
		_ = graph.UpdateEntry(id, func(e *project.BuildEntry) error {
			e.Dirty = true
			return nil
		})
		return
	}

	if entry.Hash == newHash {
		return
	}

	oldHash := entry.Hash
	_ = graph.UpdateEntry(id, func(e *project.BuildEntry) error {
		e.Hash = newHash
		e.Dirty = true
		return nil
	})

	log.DDebug(log.Handler).
		Str("id", id).
		Str("old_hash", oldHash[:8]).
		Str("new_hash", newHash[:8]).
		Msg("entry changed, marked dirty")
}

// removeEntry removes an entry from the graph and clears it from the manifest.
func (h *Handlers) removeEntry(id string) {
	graph := h.project.Graph()

	entry, ok := graph.EntryByID(id)
	if !ok {
		return
	}

	graph.RemoveNode(project.NewEntryNode(entry))
	h.project.Manifest.Delete(id)

	log.IInfo(log.Handler).Str("id", id).Msg("entry removed from graph")
}

// computeAndMarkDirty sets the initial hash on a freshly added entry and
// marks it dirty. Used only for newly created files with no prior hash.
func (h *Handlers) computeAndMarkDirty(ctx context.Context, entry *project.BuildEntry) error {
	hash, err := h.hasher.ComputeHash(ctx, entry)
	if err != nil {
		return err
	}
	return h.project.Graph().UpdateEntry(entry.ID, func(e *project.BuildEntry) error {
		e.Hash = hash
		e.Dirty = true
		return nil
	})
}

// hashNewEntries hashes all entries with an empty hash. Used after
// PartialScan adds fresh nodes that have no hash yet.
func (h *Handlers) hashNewEntries(ctx context.Context) {
	for _, entry := range h.project.Graph().AllEntries() {
		if entry.Hash == "" {
			if err := h.computeAndMarkDirty(ctx, entry); err != nil {
				log.WWarn(log.Handler).Err(err).Str("id", entry.ID).Msg("failed to hash new entry")
			}
		}
	}
}

// invalidateAndRehash invalidates the compiled layout cache for a layout or
// fragment path, then rehashes all graph entries that depend on it.
// Set fromFragment=true when path is a fragment rather than a layout file.
func (h *Handlers) invalidateAndRehash(ctx context.Context, path string, fromFragment bool) {
	graph := h.project.Graph()

	var layoutNodes []project.Node

	if fromFragment {
		fragNode, ok := graph.GetNode(fmt.Sprintf("fragment:%s", path))
		if !ok {
			return
		}
		for _, n := range graph.DependentsOf(fragNode) {
			if n.Kind() == project.NodeKindLayout {
				layoutNodes = append(layoutNodes, n)
			}
		}
	} else {
		for _, n := range graph.AllNodes() {
			if n.Kind() == project.NodeKindLayout {
				if ln, ok := n.(*project.LayoutNode); ok && ln.Key().LayoutPath == path {
					layoutNodes = append(layoutNodes, n)
				}
			}
		}
	}

	for _, n := range layoutNodes {
		ln := n.(*project.LayoutNode)
		h.project.Layouts.Invalidate(ln.Key())

		for _, affected := range graph.AffectedBy(n) {
			if affected.Kind() == project.NodeKindEntry {
				if en, ok := affected.(*project.EntryNode); ok {
					h.rehashAndMark(ctx, en.Entry().ID)
				}
			}
		}
	}
}

func (h *Handlers) handleConfigModified(ctx context.Context, paths []string) error {
	cfg, err := project.LoadConfig(h.project.Config.SourcePath)
	if err != nil {
		log.EError(log.Handler).Err(err).Msg("failed to reload config")
		return err
	}

	h.project.Ignore.Reload(cfg.Ignore)

	h.project.PipesRegistry.UnloadAll()
	for _, name := range cfg.Pipes {
		if err := h.project.PipesRegistry.Load(name); err != nil {
			log.WWarn(log.Handler).Err(err).Str("pipe", name).Msg("pipe reload failed")
		}
	}
	h.project.Config.Pipes = cfg.Pipes

	for _, entry := range h.project.Graph().AllEntries() {
		_ = h.project.Graph().UpdateEntry(entry.ID, func(e *project.BuildEntry) error {
			e.Dirty = true
			return nil
		})
	}

	if cfg.Layout != h.project.Config.Layout {
		log.IInfo(log.Handler).
			Str("old", h.project.Config.Layout).
			Str("new", cfg.Layout).
			Msg("layout glob changed, invalidating cache and rescanning")
		h.project.Layouts.InvalidateAll()
		h.project.Layouts.SetNamePattern(cfg.Layout)
		h.project.Config.Layout = cfg.Layout
		h.project.Graph().Clear()
		if err := h.source.Scan(ctx, h.project.Graph()); err != nil {
			log.EError(log.Handler).Err(err).Msg("rescan after layout glob change failed")
		} else {
			h.hashNewEntries(ctx)
		}
	}

	h.project.Config.Variables = cfg.Variables
	h.project.Config.Build = cfg.Build

	log.DDebug(log.Handler).
		Interface("cfg.Server", cfg.Server).
		Interface("cur.Server", h.project.Config.Server).
		Msg("config comparison")

	if cfg.Server != h.project.Config.Server {
		log.WWarn(log.Handler).Msg("server config changed; restart required")
	}

	log.IInfo(log.Handler).Msg("config reloaded")
	return nil
}
