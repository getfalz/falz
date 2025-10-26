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
)

type EntryType string

const (
	EntryTypeContent EntryType = "content"
	EntryTypeAsset   EntryType = "asset"
)

type SourceType string

const (
	SourceTypeSource SourceType = "source"
	SourceTypeScheme SourceType = "scheme"
)

// BuildEntry represents any file to be built (content or asset)
type BuildEntry struct {
	ID         string     // Unique identifier: "content/about.md" or "assets/style.css"
	Type       EntryType  // "content" or "asset"
	SourceRoot SourceType // Source root: "source" or "scheme"
	SourcePath string     // Source file path
	OutputPath string     // Where to write the result

	// Hash components
	Hash           string            // Combined hash of all inputs
	ContentHash    string            // Hash of source file
	LayoutHash     string            // Hash of layout (empty for assets)
	PipelineHash   string            // Hash of pipeline config
	FragmentHashes map[string]string // Fragment dependencies (empty for assets)

	// Build information
	Layout    *layout.Layout // nil for assets
	LayoutKey LayoutKey      // empty for assets
	Pipeline  string         // Pipeline name
	TagName   string         // Content slot name (empty for assets)
	BaseDir   string         // Base directory

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

	manifest, err := LoadManifest(filepath.Join(p.Config.OutputPath, manifestoFile))
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to load manifest: %v", err))
		manifest = NewManifest()
	}

	var cas *CAS
	if p.Config.Build.UseCAS {
		cas, err = NewCAS(p.Config.Build.CASPath)
		if err != nil {
			return nil, fmt.Errorf("create CAS: %w", err)
		}
		log.TTrace(log.Plan).
			Str("path", p.Config.Build.CASPath).
			Msg("CAS enabled")
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

// TODO could replace HasEntry
func (p *Plan) Entry(key string) (*BuildEntry, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	e, ok := p.entries[key]
	return e, ok
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

func (p *Project) createEntry(
	entryType EntryType,
	sourceType SourceType,
	rel string,
	base string,
	pipeName string,
	tagName string,
	lyt *layout.Layout,
) *BuildEntry {

	id := rel
	if entryType == "asset" {
		switch sourceType {
		case "scheme":
			id = fmt.Sprintf("asset:scheme:%s", id)
		case "source":
			id = fmt.Sprintf("asset:source:%s", id)
		}
	}

	e := &BuildEntry{
		ID:             id,
		Type:           entryType,
		SourcePath:     rel,
		SourceRoot:     sourceType,
		OutputPath:     p.ComputeOutputPath(base, rel, pipeName),
		Pipeline:       pipeName,
		TagName:        tagName,
		BaseDir:        base,
		FragmentHashes: make(map[string]string),
	}

	if entryType == "content" && lyt != nil {
		key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}

		e.Layout = lyt
		e.LayoutKey = key
	}

	return e
}

func (p *Project) CreateEntryWithHash(
	ctx context.Context,
	plan *Plan,
	entryType EntryType,
	sourceType SourceType,
	rel string,
	base string,
	pipeName string,
	tagName string,
	lyt *layout.Layout,
) (*BuildEntry, error) {

	entry := p.createEntry(entryType, sourceType, rel, base, pipeName, tagName, lyt)

	if err := p.computeEntryHash(ctx, entry, plan); err != nil {
		return nil, fmt.Errorf("compute hash for %s: %w", rel, err)
	}
	return entry, nil
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
				for _, cc := range lyt.Contents() {
					glob := filepath.ToSlash(filepath.Join(base, cc.Source))
					matches, err := fs.Glob(p.SourceRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						entry := p.createEntry("content", "source", rel, base, cc.Pipe, cc.Name, lyt)

						plan.AddEntry(entry.ID, entry)
						changed++
					}

					for _, a := range cc.Assets() {
						glob := filepath.ToSlash(filepath.Join(base, a.Source))
						matches, err := fs.Glob(p.SourceRoot.FS(), glob)
						if err != nil {
							continue
						}

						for _, rel := range matches {
							entry := p.createEntry("asset", "source", rel, filepath.Dir(rel), a.Pipe, a.Name, nil)

							plan.AddEntry(entry.ID, entry)
							changed++
						}
					}

				}

				for _, a := range lyt.Assets() {
					glob := filepath.ToSlash(filepath.Join(base, a.Source))
					matches, err := fs.Glob(p.SchemeRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						entry := p.createEntry("asset", "scheme", rel, filepath.Dir(rel), a.Pipe, a.Name, nil)

						plan.AddEntry(entry.ID, entry)

						changed++
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

			base := filepath.ToSlash(path)
			lmap, err := p.getLayoutsForDir(path)
			if base != startDir {
				if err == nil && len(lmap) > 0 {
					return fs.SkipDir
				}
			}

			for _, lyt := range lmap {
				key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}

				_, err := p.Layouts.GetOrBuild(ctx, p.SchemeRoot, key, lyt)
				if err != nil {
					log.Warn(fmt.Sprintf("warm up layout %s: %v", key.RelLayoutPath, err))
					continue
				}

				for _, cc := range lyt.Contents() {
					glob := filepath.ToSlash(filepath.Join(base, cc.Source))
					matches, err := fs.Glob(p.SourceRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						if err := p.updateOrCreateContentEntry(ctx, plan, rel, lyt, key, cc); err != nil {
							log.Warn(fmt.Sprintf("updating or creating content %s: %v", rel, err))
							continue
						}
					}

					for _, a := range cc.Assets() {
						glob := filepath.ToSlash(filepath.Join(base, a.Source))
						matches, err := fs.Glob(p.SourceRoot.FS(), glob)
						if err != nil {
							continue
						}

						for _, rel := range matches {
							id := fmt.Sprintf("asset:source:%s", rel)
							if err := p.updateOrCreateAssetEntry(ctx, plan, id, rel, "source", a.Pipe, filepath.Dir(rel)); err != nil {
								log.Warn(fmt.Sprintf("updating or creating asset %s: %v", rel, err))
								continue

							}
						}
					}
				}

				for _, a := range lyt.Assets() {
					glob := filepath.ToSlash(filepath.Join(base, a.Source))
					matches, err := fs.Glob(p.SchemeRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						id := fmt.Sprintf("asset:scheme:%s", rel)
						if err := p.updateOrCreateAssetEntry(ctx, plan, id, rel, "scheme", a.Pipe, filepath.Dir(rel)); err != nil {
							log.Warn(fmt.Sprintf("updating or creating asset %s: %v", rel, err))
						}
					}
				}
			}
		}

		return nil
	})
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

			if err := project.computeEntryHash(ctx, e, p); err != nil {
				errChan <- err
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

func (p *Project) computeEntryHash(ctx context.Context, e *BuildEntry, plan *Plan) error {
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

// updateEntryHashes recomputes all hashes for an entry
func (p *Project) updateEntryHashes(ctx context.Context, entry *BuildEntry, plan *Plan) error {
	oldHash := entry.Hash

	if err := p.computeEntryHash(ctx, entry, plan); err != nil {
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

func (p *Plan) GetAllEntries() []*BuildEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var all []*BuildEntry
	for _, entry := range p.entries {
		all = append(all, entry)
	}
	return all
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

func (p *Project) updateOrCreateContentEntry(
	ctx context.Context,
	plan *Plan,
	rel string,
	lyt *layout.Layout,
	key LayoutKey,
	cc layout.Content,
) error {

	entry, exists := plan.Entry(rel)

	if exists && entry.LayoutKey == key {
		err := plan.UpdateEntry(rel, func(e *BuildEntry) error {
			return p.updateEntryHashes(ctx, e, plan)
		})

		if err != nil {
			log.WWarn(log.Plan).
				Err(err).
				Str("id", rel).
				Msg("hash update failed, recreating entry")
		} else {
			return nil
		}
	}

	if exists {
		plan.DeleteEntry(rel)
	}

	entry, err := p.CreateEntryWithHash(ctx, plan, "content", "source", rel, key.BaseDir, cc.Pipe, cc.Name, lyt)
	if err != nil {
		return err
	}

	entry.Dirty = true
	plan.AddEntry(entry.ID, entry)

	log.DDebug(log.Plan).
		Str("id", rel).
		Str("layout", key.RelLayoutPath).
		Msg("new content entry added")

	return nil
}

// updateOrCreateAssetEntry updates an existing asset entry or creates a new one
func (p *Project) updateOrCreateAssetEntry(
	ctx context.Context,
	plan *Plan,
	id string,
	rel string,
	sourceRoot SourceType,
	pipeline string,
	baseDir string,
) error {
	_, exists := plan.Entry(id)

	if exists {
		err := plan.UpdateEntry(id, func(e *BuildEntry) error {
			return p.updateEntryHashes(ctx, e, plan)
		})

		if err != nil {
			log.WWarn(log.Plan).
				Err(err).
				Str("id", id).
				Msg("hash update failed, recreating entry")
		} else {
			return nil
		}
	}

	entry, err := p.CreateEntryWithHash(ctx, plan, "asset", sourceRoot, rel, baseDir, pipeline, "", nil)
	if err != nil {
		return err
	}

	entry.Dirty = true
	plan.AddEntry(entry.ID, entry)

	log.DDebug(log.Plan).
		Str("id", id).
		Str("pipeline", pipeline).
		Msg("new asset entry added")

	return nil
}
