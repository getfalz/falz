package project

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"craeft/internal/layout"
	"craeft/internal/log"

	"golang.org/x/sync/errgroup"
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

	total, err := p.scanAllFiles(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("scan files: %w", err)
	}

	if err := plan.computeAllHashes(p); err != nil {
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

// scanAllFiles walks directories and creates build entries for content and assets
func (p *Project) scanAllFiles(ctx context.Context, plan *Plan) (int, error) {
	changed, err := p.scanContentFiles(plan)
	if err != nil {
		return 0, fmt.Errorf("scan content: %w", err)
	}

	assetChanged, err := p.scanAssetFiles(plan)
	if err != nil {
		return 0, fmt.Errorf("scan assets: %w", err)
	}

	return changed + assetChanged, nil
}

// scanContentFiles walks the source directory for content
func (p *Project) scanContentFiles(plan *Plan) (int, error) {
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

				// Process content definitions
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
					}
				}

				// Process assets in content
				for _, cc := range lyt.Contents() {
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
						}
					}
				}
			}
		}
		return nil
	})

	return changed, err
}

// scanAssetFiles scans for layout-level assets
func (p *Project) scanAssetFiles(plan *Plan) (int, error) {
	// Walk scheme directory for layout assets
	changed := 0
	err := fs.WalkDir(p.schemeRoot.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || !d.IsDir() {
			return walkErr
		}

		base := filepath.ToSlash(path)
		lmap, err := p.getLayouts(base)
		if err != nil {
			return nil // No layouts here
		}

		for _, lyt := range lmap {
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
				}
			}
		}
		return nil
	})

	return changed, err
}

// computeAllHashes calculates hashes for all entries
func (plan *Plan) computeAllHashes(p *Project) error {
	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(10)

	// TODO:
	// check for layout cachen and so..
	for _, entry := range plan.Entries {
		e := entry
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
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

			// Hash source file
			contentHash, err := plan.HashCache.HashFile(fsys, e.SourcePath)
			if err != nil {
				e.Error = fmt.Errorf("hash source: %w", err)
				return nil
			}
			e.ContentHash = contentHash

			// Hash layout and fragments (only for content)
			if e.Type == "content" && e.Layout != nil {
				// Hash layout
				layoutData := []byte(e.Layout.Path())
				e.LayoutHash = HashBytes(layoutData)

				// Hash fragments
				for _, frag := range e.Layout.Fragments() {
					fragHash, err := plan.HashCache.HashFile(p.schemeRoot.FS(), frag)
					if err != nil {
						log.Warn(fmt.Sprintf("Failed to hash fragment %s: %v", frag, err))
						continue
					}
					e.FragmentHashes[frag] = fragHash
				}
			}

			// Hash pipeline configuration
			if pl, err := p.pipelines.Get(e.Pipeline); err == nil {
				cfg := pl.Config()
				e.PipelineHash = HashString(fmt.Sprintf("%s-%s-%s",
					cfg.PipelineName, cfg.SourceSuffix, cfg.TargetSuffix))
			}

			// Compute combined hash
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
		})
	}

	return g.Wait()
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
