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
	Entries   map[string]*BuildEntry // All entries (content and assets)
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
		Entries:   make(map[string]*BuildEntry),
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

	log.IInfo(log.Plan).
		Int("total", total).
		Int("dirty", changed).
		Dur("duration", time.Since(start)).
		Msg("incremental plan generated")

	return plan, nil
}

// scanFiles walks the source directory for content
func (p *Project) scanFiles(plan *Plan) (int, error) {
	srcFS := p.srcRoot.FS()

	changed := 0
	err := fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if p.ignoreFile(path) {
				return fs.SkipDir
			}

			base := filepath.ToSlash(path)
			lmap, err := p.getLayouts(base)
			if err != nil {
				log.TTrace(log.Layout).Str("base", base).Err(err).Msg("no layouts for base")
				return nil
			}

			for _, lyt := range lmap {
				key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}

				for _, cc := range lyt.Contents() {
					glob := filepath.ToSlash(filepath.Join(base, cc.Path))
					matches, err := fs.Glob(p.srcRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						entry := &BuildEntry{
							ID:             rel,
							Type:           "content",
							SourcePath:     rel,
							SourceRoot:     "source",
							OutputPath:     p.computeOutputPath(base, rel, cc.Pipe),
							Layout:         lyt,
							LayoutKey:      key,
							Pipeline:       cc.Pipe,
							ContentName:    cc.Name,
							BaseDir:        base,
							FragmentHashes: make(map[string]string),
						}

						plan.mu.Lock()
						plan.Entries[entry.ID] = entry
						changed++
						plan.mu.Unlock()

						log.TTrace(log.Plan).
							Str("layout", key.RelLayoutPath).
							Str("content", rel).
							Str("pipe", cc.Pipe).
							Msg("page planned")
					}

					for _, a := range cc.Assets() {
						glob := filepath.ToSlash(filepath.Join(base, a.Source))
						matches, err := fs.Glob(p.srcRoot.FS(), glob)
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
								OutputPath:     p.computeOutputPath(filepath.Dir(rel), rel, a.AssetType),
								Pipeline:       a.AssetType,
								BaseDir:        filepath.Dir(rel),
								FragmentHashes: make(map[string]string),
							}

							plan.mu.Lock()
							plan.Entries[entry.ID] = entry
							changed++
							plan.mu.Unlock()

							log.TTrace(log.Plan).
								Str("from", "source").
								Str("pipe", a.AssetType).
								Str("rel", rel).
								Msg("asset planned")
						}
					}

				}

				for _, a := range lyt.Assets() {
					glob := filepath.ToSlash(filepath.Join(lyt.Path(), a.Source))
					matches, err := fs.Glob(p.schemeRoot.FS(), glob)
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
							OutputPath:     p.computeOutputPath(filepath.Dir(rel), rel, a.AssetType),
							Pipeline:       a.AssetType,
							BaseDir:        filepath.Dir(rel),
							FragmentHashes: make(map[string]string),
						}

						plan.mu.Lock()
						plan.Entries[entry.ID] = entry
						changed++
						plan.mu.Unlock()

						log.TTrace(log.Plan).
							Str("from", "scheme").
							Str("pipe", a.AssetType).
							Str("rel", rel).
							Msg("asset planned")
					}
				}

			}
		}
		return nil
	})

	return changed, err
}

func (p *Project) warmupLayouts(ctx context.Context, plan *Plan) error {
	layoutKeys := make(map[LayoutKey]struct{})
	for _, entry := range plan.Entries {
		if entry.Type == "content" && entry.Layout != nil {
			layoutKeys[entry.LayoutKey] = struct{}{}
		}
	}

	log.IInfo(log.Layout).
		Int("layouts", len(layoutKeys)).
		Msg("warming up layout cache")

	var errs []error
	for key := range layoutKeys {
		lmap, err := p.getLayouts(key.BaseDir)
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

		_, err = p.layouts.GetOrBuild(ctx, p.schemeRoot, key, lyt)
		if err != nil {
			errs = append(errs, fmt.Errorf("build layout %s: %w", key.RelLayoutPath, err))
		}
	}

	return errors.Join(errs...)
}

// computeHashes calculates hashes for all entries
func (plan *Plan) computeHashes(ctx context.Context, p *Project) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	errChan := make(chan error, len(plan.Entries))

	for _, entry := range plan.Entries {
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

			if err := p.computeEntryHash(ctx, e, plan); err != nil {
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

func (p *Project) computeEntryHash(ctx context.Context, e *BuildEntry, plan *Plan) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var fsys fs.FS
	switch e.SourceRoot {
	case "source":
		fsys = p.srcRoot.FS()
	case "scheme":
		fsys = p.schemeRoot.FS()
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
		layoutHash, err := plan.HashCache.HashFile(p.schemeRoot.FS(), layoutPath)
		if err != nil {
			e.Error = fmt.Errorf("hash layout %s: %w", layoutPath, err)
			return e.Error
		}
		e.LayoutHash = layoutHash

		deps, ok := p.layouts.GetDeps(e.LayoutKey)
		if !ok {
			e.Error = fmt.Errorf("failed to get layout dependencies for %s", layoutPath)
			return e.Error
		}

		for _, frag := range deps {
			fragHash, err := plan.HashCache.HashFile(p.schemeRoot.FS(), frag)
			if err != nil {
				e.Error = fmt.Errorf("hash fragment %s: %w", frag, err)
				return e.Error
			}
			e.FragmentHashes[frag] = fragHash
		}
	}

	if pl, err := p.pipelines.Get(e.Pipeline); err == nil {
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

	log.TTrace(log.Plan).
		Str("id", e.ID).
		Str("type", e.Type).
		Str("hash", e.Hash[:8]).
		Msg("computed hash")

	return nil
}

// diff compares the plan against the manifest
func (plan *Plan) diff() int {
	plan.mu.Lock()
	defer plan.mu.Unlock()

	changed := 0
	for id, entry := range plan.Entries {
		if plan.Manifest.IsDirty(id, entry.Hash) {
			entry.Dirty = true
			changed++

			log.TTrace(log.Plan).
				Str("id", id).
				Str("type", entry.Type).
				Msg("marked dirty")
		}
	}

	return changed
}

// GetDirtyEntries returns all entries that need rebuilding
func (plan *Plan) GetDirtyEntries() []*BuildEntry {
	plan.mu.RLock()
	defer plan.mu.RUnlock()

	var dirty []*BuildEntry
	for _, entry := range plan.Entries {
		if entry.Dirty {
			dirty = append(dirty, entry)
		}
	}
	return dirty
}

// GetEntriesByType returns entries filtered by type
func (plan *Plan) GetEntriesByType(entryType string) []*BuildEntry {
	plan.mu.RLock()
	defer plan.mu.RUnlock()

	var entries []*BuildEntry
	for _, entry := range plan.Entries {
		if entry.Type == entryType {
			entries = append(entries, entry)
		}
	}
	return entries
}

// computeOutputPath determines the output path for a file
func (p *Project) computeOutputPath(base, rel, pipe string) string {
	pl, err := p.pipelines.Get(pipe)
	if err != nil {
		return filepath.Join(base, filepath.Base(rel))
	}

	cfg := pl.Config()
	target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(rel), "."+cfg.TargetSuffix)
	return filepath.Join(base, target)
}
