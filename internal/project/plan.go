package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"craeft/internal/layout"
	"craeft/internal/log"

	"github.com/valyala/fasttemplate"
)

// BuildEntry represents any file to be built (content or asset)
type BuildEntry struct {
	ID         string // Unique identifier: "content/about.md" or "assets/style.css"
	Type       string // "content" or "asset"
	SourcePath string // Source file path
	SourceRoot string // "source" or "scheme"
	OutputPath string // Where to write the result

	// Hash components
	Hash           string            // Combined hash of all inputs
	ContentHash    string            // Hash of source file
	LayoutHash     string            // Hash of layout (empty for assets)
	PipelineHash   string            // Hash of pipeline config
	FragmentHashes map[string]string // Fragment dependencies (empty for assets)

	// Build information
	Layout      *layout.Layout // nil for assets
	LayoutKey   LayoutKey      // empty for assets
	Pipeline    string         // Pipeline name
	ContentName string         // Content slot name (empty for assets)
	BaseDir     string         // Base directory

	// State
	Dirty     bool      // Needs rebuilding
	Error     error     // Build error if any
	BuildTime time.Time // Last successful build
}

// Plan represents a build plan with change detection
type Plan struct {
	entries   map[string]*BuildEntry // All entries (content and assets)
	HashCache *HashCache
	Manifest  *Manifest
	CAS       *CAS

	mu sync.RWMutex
}

// Plan creates a new incremental build plan
func (p *Project) Plan(ctx context.Context) (*Plan, error) {
	start := time.Now()

	// TODO use another file name
	manifest, err := LoadManifest(filepath.Join(p.Config.OutputPath, ".craeft-manifest.json"))
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to load manifest: %v", err))
		manifest = NewManifest()
	}

	// TODO use another cache dir
	// TODO CAS is optional. respect that
	cas, err := NewCAS(filepath.Join(p.Config.OutputPath, ".craeft-cache"))
	if err != nil {
		return nil, fmt.Errorf("create CAS: %w", err)
	}

	plan := &Plan{
		entries:   make(map[string]*BuildEntry),
		HashCache: NewHashCache(),
		Manifest:  manifest,
		CAS:       cas,
	}

	total, err := p.scanFiles(plan)
	if err != nil {
		return nil, fmt.Errorf("scan files: %w", err)
	}

	if err := p.warmupLayouts(ctx, plan); err != nil {
		return nil, fmt.Errorf("warmup layouts: %w", err)
	}

	if err := plan.computeHashes(ctx, p); err != nil {
		return nil, fmt.Errorf("compute hashes: %w", err)
	}

	changed := plan.diff()

	p.currentPlanMu.Lock()
	p.currentPlan = plan
	p.currentPlanMu.Unlock()

	log.IInfo(log.Plan).
		Int("total", total).
		Int("dirty", changed).
		Dur("duration", time.Since(start)).
		Msg("incremental plan generated")

	return plan, nil
}
func (p *Plan) HasEntry(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.entries[key]
	return ok
}

func (p *Plan) LenEntries() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.entries)
}
func (p *Plan) AddEntry(key string, entry *BuildEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries[key] = entry
}

func (p *Plan) DeleteEntry(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, key)
}

func (p *Plan) UpdateEntry(key string, fn func(entry *BuildEntry) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.entries[key]
	if !ok {
		return fmt.Errorf("entry %q not found", key)
	}
	return fn(entry)
}

func (p *Plan) EntryKeys(predicate func(*BuildEntry) bool) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	keys := make([]string, 0)
	for k, e := range p.entries {
		if predicate(e) {
			keys = append(keys, k)
		}
	}
	return keys
}

// scanFiles walks the source directory for content
func (p *Project) scanFiles(plan *Plan) (int, error) {
	srcFS := p.SourceRoot.FS()

	changed := 0
	err := fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if p.IgnoreFile(path) {
				return fs.SkipDir
			}

			base := filepath.ToSlash(path)
			lmap, err := p.GetLayouts(base)
			if err != nil {
				return nil
			}

			for _, lyt := range lmap {
				key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}

				for _, cc := range lyt.Contents() {
					glob := filepath.ToSlash(filepath.Join(base, cc.Path))
					matches, err := fs.Glob(p.SourceRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						entry := &BuildEntry{
							ID:             rel,
							Type:           "content",
							SourcePath:     rel,
							SourceRoot:     "source",
							OutputPath:     p.ComputeOutputPath(base, rel, cc.Pipe),
							Layout:         lyt,
							LayoutKey:      key,
							Pipeline:       cc.Pipe,
							ContentName:    cc.Name,
							BaseDir:        base,
							FragmentHashes: make(map[string]string),
						}

						plan.mu.Lock()
						plan.entries[entry.ID] = entry
						changed++
						plan.mu.Unlock()
					}

					for _, a := range cc.Assets() {
						glob := filepath.ToSlash(filepath.Join(base, a.Source))
						matches, err := fs.Glob(p.SourceRoot.FS(), glob)
						if err != nil {
							continue
						}

						for _, rel := range matches {
							id := fmt.Sprintf("asset:source:%s", rel)
							entry := &BuildEntry{
								ID:             id,
								Type:           "asset",
								SourcePath:     rel,
								SourceRoot:     "source",
								OutputPath:     p.ComputeOutputPath(filepath.Dir(rel), rel, a.AssetType),
								Pipeline:       a.AssetType,
								BaseDir:        filepath.Dir(rel),
								FragmentHashes: make(map[string]string),
							}

							plan.mu.Lock()
							plan.entries[entry.ID] = entry
							changed++
							plan.mu.Unlock()
						}
					}

				}

				for _, a := range lyt.Assets() {
					glob := filepath.ToSlash(filepath.Join(lyt.Path(), a.Source))
					matches, err := fs.Glob(p.SchemeRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						id := fmt.Sprintf("asset:scheme:%s", rel)
						entry := &BuildEntry{
							ID:             id,
							Type:           "asset",
							SourcePath:     rel,
							SourceRoot:     "scheme",
							OutputPath:     p.ComputeOutputPath(filepath.Dir(rel), rel, a.AssetType),
							Pipeline:       a.AssetType,
							BaseDir:        filepath.Dir(rel),
							FragmentHashes: make(map[string]string),
						}

						plan.mu.Lock()
						plan.entries[entry.ID] = entry
						changed++
						plan.mu.Unlock()
					}
				}

			}
		}
		return nil
	})

	return changed, err
}

// PartialScan walks a directory tree and updates plan entries
// Stops at subdirectories with their own layouts
func (p *Project) PartialScan(ctx context.Context, plan *Plan, baseDir string) error {
	srcFS := p.SourceRoot.FS()

	log.DDebug(log.Plan).
		Str("base", baseDir).
		Msg("partial scan starting")

	startDir := baseDir

	return fs.WalkDir(srcFS, startDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			if p.IgnoreFile(path) {
				return fs.SkipDir
			}

			lmap, err := p.getLayoutsForDir(path)
			if err == nil && len(lmap) > 0 {
				return fs.SkipDir
			}

			base := filepath.ToSlash(path)
			if err != nil {
				return nil
			}

			for _, lyt := range lmap {
				key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}

				// TODO check this warmup?
				_, err := p.Layouts.GetOrBuild(ctx, p.SchemeRoot, key, lyt)
				if err != nil {
					log.Warn(fmt.Sprintf("warm up layout %s: %v", key.RelLayoutPath, err))
					continue
				}

				for _, cc := range lyt.Contents() {
					glob := filepath.ToSlash(filepath.Join(base, cc.Path))
					matches, err := fs.Glob(p.SourceRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						p.updateOrCreateContentEntry(ctx, plan, rel, lyt, key, cc)
					}

					for _, a := range cc.Assets() {
						glob := filepath.ToSlash(filepath.Join(base, a.Source))
						matches, err := fs.Glob(p.SourceRoot.FS(), glob)
						if err != nil {
							continue
						}

						for _, rel := range matches {
							id := fmt.Sprintf("asset:source:%s", rel)
							p.updateOrCreateAssetEntry(ctx, plan, id, rel, "source", a.AssetType, filepath.Dir(rel))
						}
					}
				}

				for _, a := range lyt.Assets() {
					glob := filepath.ToSlash(filepath.Join(lyt.Path(), a.Source))
					matches, err := fs.Glob(p.SchemeRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						id := fmt.Sprintf("asset:scheme:%s", rel)
						p.updateOrCreateAssetEntry(ctx, plan, id, rel, "scheme", a.AssetType, filepath.Dir(rel))
					}
				}
			}
		}

		return nil
	})
}
func (p *Project) WarmUpLayout(ctx context.Context, key LayoutKey, lyt *layout.Layout) (*fasttemplate.Template, error) {
	return p.Layouts.GetOrBuild(ctx, p.SchemeRoot, key, lyt)
}

func (p *Project) warmupLayouts(ctx context.Context, plan *Plan) error {
	layoutKeys := make(map[LayoutKey]struct{})
	for _, entry := range plan.entries {
		if entry.Type == "content" && entry.Layout != nil {
			layoutKeys[entry.LayoutKey] = struct{}{}
		}
	}

	log.IInfo(log.Layout).
		Int("layouts", len(layoutKeys)).
		Msg("warming up layout cache")

	var errs []error
	for key := range layoutKeys {
		lmap, err := p.GetLayouts(key.BaseDir)
		if err != nil {
			errs = append(errs, fmt.Errorf("get layouts for %s: %w", key.BaseDir, err))
			continue
		}

		keyPath := filepath.ToSlash(key.RelLayoutPath)
		lyt, ok := lmap[keyPath]
		if !ok {
			errs = append(errs, fmt.Errorf("layout not found: %s", keyPath))
			continue
		}

		_, err = p.Layouts.GetOrBuild(ctx, p.SchemeRoot, key, lyt)
		if err != nil {
			errs = append(errs, fmt.Errorf("build layout %s: %w", key.RelLayoutPath, err))
		}
	}

	return errors.Join(errs...)
}

// computeHashes calculates hashes for all entries
func (p *Plan) computeHashes(ctx context.Context, project *Project) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	errChan := make(chan error, len(p.entries))

	for _, entry := range p.entries {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(e *BuildEntry) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}

			if err := project.ComputeEntryHash(ctx, e, p); err != nil {
				if ctx.Err() != nil {
					return
				}
				select {
				case errChan <- err:
				default:
					log.EError(log.Build).
						Err(err).
						Msg("BUG: error channel full despite being sized to len(entries)")
				}
			}
		}(entry)
	}

	wg.Wait()
	close(errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (p *Project) ComputeEntryHash(ctx context.Context, e *BuildEntry, plan *Plan) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var fsys fs.FS
	switch e.SourceRoot {
	case "source":
		fsys = p.SourceRoot.FS()
	case "scheme":
		fsys = p.SchemeRoot.FS()
	default:
		e.Error = fmt.Errorf("unknown source root: %s", e.SourceRoot)
		return nil
	}

	contentHash, err := plan.HashCache.HashFile(fsys, e.SourcePath)
	if err != nil {
		e.Error = fmt.Errorf("hash source: %w", err)
		return e.Error
	}
	e.ContentHash = contentHash

	// For content entries, get layout and fragment hashes from cache
	if e.Type == "content" && e.Layout != nil {
		layoutPath := e.Layout.Path()
		layoutHash, err := plan.HashCache.HashFile(p.SchemeRoot.FS(), layoutPath)
		if err != nil {
			e.Error = fmt.Errorf("hash layout %s: %w", layoutPath, err)
			return e.Error
		}
		e.LayoutHash = layoutHash

		deps, ok := p.Layouts.GetDeps(e.LayoutKey)
		if !ok {
			e.Error = fmt.Errorf("failed to get layout dependencies for %s", layoutPath)
			return e.Error
		}

		for _, frag := range deps {
			fragHash, err := plan.HashCache.HashFile(p.SchemeRoot.FS(), frag)
			if err != nil {
				e.Error = fmt.Errorf("hash fragment %s: %w", frag, err)
				return e.Error
			}
			e.FragmentHashes[frag] = fragHash
		}
	}

	if pl, err := p.Pipes.Get(e.Pipeline); err == nil {
		cfg := pl.Config()
		e.PipelineHash = HashString(fmt.Sprintf("%s-%s-%s",
			cfg.PipelineName, cfg.SourceSuffix, cfg.TargetSuffix))
	}

	entryHash := EntryHash{
		ContentHash:    e.ContentHash,
		LayoutHash:     e.LayoutHash,
		PipelineHash:   e.PipelineHash,
		FragmentHashes: e.FragmentHashes,
	}
	e.Hash = entryHash.Compute()

	return nil
}

// diff compares the plan against the manifest
func (p *Plan) diff() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	changed := 0
	for id, entry := range p.entries {
		if p.Manifest.IsDirty(id, entry.Hash) {
			entry.Dirty = true
			changed++
		}
	}

	return changed
}

// GetDirtyEntries returns all entries that need rebuilding
func (p *Plan) GetDirtyEntries() []*BuildEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var dirty []*BuildEntry
	for _, entry := range p.entries {
		if entry.Dirty {
			dirty = append(dirty, entry)
		}
	}
	return dirty
}

// GetEntriesByType returns entries filtered by type
func (p *Plan) GetEntriesByType(entryType string) []*BuildEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var entries []*BuildEntry
	for _, entry := range p.entries {
		if entry.Type == entryType {
			entries = append(entries, entry)
		}
	}
	return entries
}

// ComputeOutputPath determines the output path for a file
func (p *Project) ComputeOutputPath(base, rel, pipe string) string {
	pl, err := p.Pipes.Get(pipe)
	if err != nil {
		return filepath.Join(base, filepath.Base(rel))
	}

	cfg := pl.Config()
	target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(rel), "."+cfg.TargetSuffix)
	return filepath.Join(base, target)
}

// updateOrCreateContentEntry updates an existing content entry or creates a new one
func (p *Project) updateOrCreateContentEntry(
	ctx context.Context,
	plan *Plan,
	rel string,
	lyt *layout.Layout,
	key LayoutKey,
	cc layout.Content,
) {
	plan.mu.Lock()
	existingEntry, exists := plan.entries[rel]
	plan.mu.Unlock()

	if exists && existingEntry.LayoutKey == key {
		// Entry exists with correct layout, just update hashes
		if err := p.updateEntryHashes(ctx, existingEntry, plan); err != nil {
			log.Warn(fmt.Sprintf("update entry hash %s: %v", rel, err))
		}
	} else {
		// Entry doesn't exist or has wrong layout
		if exists {
			plan.mu.Lock()
			delete(plan.entries, rel)
			plan.mu.Unlock()
		}

		// Create new entry
		entry := &BuildEntry{
			ID:             rel,
			Type:           "content",
			SourcePath:     rel,
			SourceRoot:     "source",
			OutputPath:     p.ComputeOutputPath(key.BaseDir, rel, cc.Pipe),
			Layout:         lyt,
			LayoutKey:      key,
			Pipeline:       cc.Pipe,
			ContentName:    cc.Name,
			BaseDir:        key.BaseDir,
			FragmentHashes: make(map[string]string),
		}

		if err := p.ComputeEntryHash(ctx, entry, plan); err != nil {
			log.Warn(fmt.Sprintf("compute entry hash %s: %v", rel, err))
			return
		}

		entry.Dirty = true

		plan.mu.Lock()
		plan.entries[entry.ID] = entry
		plan.mu.Unlock()

		log.DDebug(log.Plan).
			Str("id", rel).
			Str("layout", key.RelLayoutPath).
			Msg("new content entry added")
	}
}

// updateOrCreateAssetEntry updates an existing asset entry or creates a new one
func (p *Project) updateOrCreateAssetEntry(
	ctx context.Context,
	plan *Plan,
	id string,
	rel string,
	sourceRoot string,
	pipeline string,
	baseDir string,
) {
	plan.mu.Lock()
	existingEntry, exists := plan.entries[id]
	plan.mu.Unlock()

	if exists {
		// Entry exists, update hashes
		if err := p.updateEntryHashes(ctx, existingEntry, plan); err != nil {
			log.Warn(fmt.Sprintf("update asset hash %s: %v", rel, err))
		}
	} else {
		// Create new entry
		entry := &BuildEntry{
			ID:             id,
			Type:           "asset",
			SourcePath:     rel,
			SourceRoot:     sourceRoot,
			OutputPath:     p.ComputeOutputPath(baseDir, rel, pipeline),
			Pipeline:       pipeline,
			BaseDir:        baseDir,
			FragmentHashes: make(map[string]string),
		}

		if err := p.ComputeEntryHash(ctx, entry, plan); err != nil {
			log.Warn(fmt.Sprintf("compute asset hash %s: %v", rel, err))
			return
		}

		entry.Dirty = true

		plan.mu.Lock()
		plan.entries[entry.ID] = entry
		plan.mu.Unlock()

		log.DDebug(log.Plan).
			Str("id", id).
			Str("pipeline", pipeline).
			Msg("new asset entry added")
	}
}

// updateEntryHashes recomputes all hashes for an entry
func (p *Project) updateEntryHashes(ctx context.Context, entry *BuildEntry, plan *Plan) error {
	oldHash := entry.Hash

	if err := p.ComputeEntryHash(ctx, entry, plan); err != nil {
		return err
	}

	if entry.Hash != oldHash {
		entry.Dirty = true
		log.DDebug(log.Plan).
			Str("id", entry.ID).
			Str("old_hash", oldHash[:8]).
			Str("new_hash", entry.Hash[:8]).
			Msg("hash changed, marked for rebuild")
	}

	return nil
}

// RemoveIf removes entries matching the predicate and optionally updates the manifest
// Returns the IDs of removed entries
func (p *Plan) RemoveIf(predicate func(*BuildEntry) bool, updateManifest bool) []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	removed := []string{}
	for id, entry := range p.entries {
		if predicate(entry) {
			delete(p.entries, id)
			if updateManifest {
				p.Manifest.Delete(id)
			}
			removed = append(removed, id)
		}
	}

	return removed
}
