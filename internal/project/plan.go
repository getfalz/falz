package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"craeft/internal/layout"
	"craeft/internal/log"
	"craeft/internal/utils"

	"github.com/valyala/fasttemplate"
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

// EntrySpec is used to create or update an Entry
type EntrySpec struct {
	Type       EntryType

	SourceRoot SourceType
	Rel        string
	ToTpl      string
	BaseDir    string

	Pipeline   string
	TagName    string

	Layout     *layout.Layout
	LayoutKey  LayoutKey
}


// Entry represents any file to be built (content or asset)
type Entry struct {
	ID         string     // Unique identifier: "content/about.md" or "assets/style.css"
	Type       EntryType  // "content" or "asset"
	SourceRoot SourceType // Source root: "source" or "scheme"
	SourcePath string     // Source file path

	// OutputPath
	outputPathOnce sync.Once
	outputPath     string // Where to write the result
	outputPathErr  error

	// Hash components
	Hash string // Combined hash of all inputs
	// TODO replace all this
	ContentHash    string   // Hash of source file
	LayoutHash     string   // Hash of layout (empty for assets)
	PipelineHash   string   // Hash of pipeline config
	FragmentHashes []string // Fragment dependencies (empty for assets)

	// Build information
	Layout    *layout.Layout // nil for assets
	LayoutKey LayoutKey      // empty for assets
	Pipeline  string         // Pipeline name
	TagName   string         // Content slot name (empty for assets)
	BaseDir   string         // Base directory
	ToTpl     string         // Output path template

	// State
	Dirty     bool      // Needs rebuilding
	Error     error     // Build error if any
	BuildTime time.Time // Last successful build
}

var hasHashTag = regexp.MustCompile(`^HASH:(\d+)$`)

func (e *Entry) OutputPath() (string, error) {
	e.outputPathOnce.Do(func() {
		tpl := e.ToTpl

		name := filepath.Base(e.SourcePath)
		extWithDot := filepath.Ext(name)
		filename := strings.TrimSuffix(name, extWithDot)
		ext := strings.TrimPrefix(extWithDot, ".")

		if tpl == "" {
			e.outputPathErr = fmt.Errorf("missing required 'to' template for %s", e.ID)
			return
		}
		// TODO better way to check for path separators?
		if strings.ContainsAny(tpl, "/\\") {
			e.outputPathErr = fmt.Errorf("'to' must be a file name only (no dirs): %q", tpl)
			return
		}

		// TODO don't panic on template parse
		// e.g. panic: unexpected error: unknown placeholder: FILENAM
		output := fasttemplate.New(tpl, "{", "}").ExecuteFuncString(func(w io.Writer, tag string) (int, error) {
			switch tag {
			case "FILENAME":
				return w.Write([]byte(filename))
			case "EXT":
				return w.Write([]byte(ext))
			case "BASENAME":
				return w.Write([]byte(name))
			case "PIPE":
				return w.Write([]byte(e.Pipeline))
			case "HASH":
				if e.Hash == "" {
					return 0, fmt.Errorf("HASH not available yet")
				}
				return w.Write([]byte(e.Hash))
			default:
				// HASH:N ?
				if m := hasHashTag.FindStringSubmatch(tag); len(m) == 2 {
					if e.Hash == "" {
						return 0, fmt.Errorf("HASH not available yet")
					}

					n, err := strconv.Atoi(m[1])
					if err != nil || n <= 0 {
						return 0, fmt.Errorf("invalid HASH length: %q", m[1])
					}
					if n > len(e.Hash) {
						n = len(e.Hash)
					}
					return w.Write([]byte(e.Hash[:n]))
				}

				return fmt.Fprintf(w, "[unknown tag %q]", tag)
			}
		})

		if output == "" || output == "." || output == ".." || strings.ContainsAny(output, "/\\") {
			e.outputPathErr = fmt.Errorf("rendered 'to' invalid: %q", output)
			return
		}
		e.outputPath = filepath.Join(e.BaseDir, output)
	})
	return e.outputPath, e.outputPathErr
}

// Plan represents a build plan with change detection
type Plan struct {
	entries map[string]*Entry // All entries (content and assets)
	project *Project

	mu sync.RWMutex
}

type Planner interface {
	// BuildPlan(changedNodes []Node) (*Plan, error)
	BuildAllPlan() (*Plan, error)
}

// Plan creates a new incremental build plan
func (p *Project) Plan(ctx context.Context) (*Plan, error) {
	start := time.Now()

	plan := &Plan{
		entries: make(map[string]*Entry),
		project: p,
	}

	// total, err := p.scanFiles(plan)
	// TODO give root folder
	total, err := p.Scan(ctx, plan, ".", true)
	if err != nil {
		return nil, fmt.Errorf("scan files: %w", err)
	}

	keys := plan.LayoutKeys()

	// if err := plan.computeHashes(ctx, p); err != nil {
	// 	return nil, fmt.Errorf("compute hashes: %w", err)
	// }

	hashed, err := p.HashDirty(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("hash dirty: %w", err)
	}

	changed := plan.diff()

	p.currentPlanMu.Lock()
	p.currentPlan = plan
	p.currentPlanMu.Unlock()

	log.IInfo(log.Plan).
		Int("total", total).
		Int("hashed", hashed).
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
func (p *Plan) Entry(key string) (*Entry, bool) {
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
func (p *Plan) AddEntry(key string, entry *Entry) {
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
func (p *Plan) RemoveIf(predicate func(*Entry) bool, updateManifest bool) []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	removed := []string{}
	for id, entry := range p.entries {
		if predicate(entry) {
			delete(p.entries, id)
			if updateManifest {
				p.project.Manifest.Delete(id)
			}
			removed = append(removed, id)
		}
	}

	return removed
}

func (p *Plan) UpdateEntry(key string, fn func(entry *Entry) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.entries[key]
	if !ok {
		return fmt.Errorf("entry %q not found", key)
	}
	return fn(entry)
}

func (p *Plan) EntryKeys(predicate func(*Entry) bool) []string {
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

func (p *Plan) LayoutKeys() []LayoutKey {
	p.mu.RLock()
	defer p.mu.RUnlock()
	uniq := make(map[LayoutKey]struct{})
	for _, e := range p.entries {
		if e.Type == "content" && e.Layout != nil {
			uniq[e.LayoutKey] = struct{}{}
		}
	}
	out := make([]LayoutKey, 0, len(uniq))
	for k := range uniq {
		out = append(out, k)
	}
	return out
}

// func (p *Project) createEntry(
// 	entryType EntryType,
// 	sourceType SourceType,
// 	sourcePath string,
// 	outputPath string,
// 	base string,
// 	pipeName string,
// 	tagName string,
// 	lyt *layout.Layout,
// ) *BuildEntry {
//
// 	id := sourcePath
// 	if entryType == "asset" {
// 		switch sourceType {
// 		case "scheme":
// 			id = fmt.Sprintf("asset:scheme:%s", id)
// 		case "source":
// 			id = fmt.Sprintf("asset:source:%s", id)
// 		}
// 	}
//
// 	e := &BuildEntry{
// 		ID:             id,
// 		Type:           entryType,
// 		SourcePath:     sourcePath,
// 		SourceRoot:     sourceType,
// 		OutputPath:     p.ComputeOutputPath(base, sourcePath, pipeName),
// 		Pipeline:       pipeName,
// 		TagName:        tagName,
// 		BaseDir:        base,
// 		FragmentHashes: make(map[string]string),
// 	}
//
// 	if entryType == "content" && lyt != nil {
// 		key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}
//
// 		e.Layout = lyt
// 		e.LayoutKey = key
// 	}
//
// 	return e
// }

// func (p *Project) CreateEntryWithHash(
// 	ctx context.Context,
// 	plan *Plan,
// 	entryType EntryType,
// 	sourceType SourceType,
// 	sourcePath string,
// 	outputPath string,
// 	baseDir string,
// 	pipeName string,
// 	tagName string,
// 	lyt *layout.Layout,
// ) (*BuildEntry, error) {
//
// 	entry := p.createEntry(entryType, sourceType, sourcePath, outputPath, baseDir, pipeName, tagName, lyt)
//
// 	if err := p.computeEntryHash(ctx, entry, plan); err != nil {
// 		return nil, fmt.Errorf("compute hash for %s: %w", sourcePath, err)
// 	}
// 	return entry, nil
// }

func (p *Project) Scan(ctx context.Context, plan *Plan, baseDir string, full bool) (int, error) {
	touched := 0
	startDir := filepath.ToSlash(baseDir)

	err := fs.WalkDir(p.SourceFS.FS(), baseDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !d.IsDir() {
			return nil
		}
		if p.IgnoreFile(path) {
			return fs.SkipDir
		}

		base := filepath.ToSlash(path)

		var (
			lmap map[string]*layout.Layout
			err  error
		)
		if full {
			// Load all layouts as usual, looking in parent folders if layout is missing
			lmap, err = p.GetLayouts(base)
			if err != nil {
				log.WWarn(log.Plan).Err(err).Str("rel", base).Msg("failed to load layout")
				return nil
			}
		} else {
			// TODO revisit this logic
			lmap, err = p.getLayoutsForDir(base)
			if base != startDir {
				if err == nil && len(lmap) > 0 {
					return fs.SkipDir
				}
			}

			// if err != nil {
			// 	log.WWarn(log.Plan).Err(err).Str("rel", base).Msg("failed to load layout")
			// 	return err
			// }
			// // Stop descending into subdirs that introduce their own layout
			// if base != startDir && err == nil && len(lmap) > 0 {
			// 	return fs.SkipDir
			// }
		}

		// For each layout at this directory, enumerate contents + assets
		for _, lyt := range lmap {
			key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}

			// Contents
			for _, cc := range lyt.Contents() {
				glob := filepath.ToSlash(filepath.Join(base, cc.Source))
				matches, err := p.SourceFS.Glob(glob)
				if err != nil {
					continue
				}
				for _, rel := range matches {
					spec := EntrySpec{
						Type:       EntryTypeContent,
						SourceRoot: SourceTypeSource,
						Rel:        rel,
						BaseDir:    base,
						Pipeline:   cc.Pipe,
						TagName:    cc.Name,
						ToTpl:      cc.To,
						Layout:     lyt,
						LayoutKey:  key,
					}
					if _, changed := plan.UpsertEntry(spec); changed {
						touched++
					}
				}

				// Per-content assets (source)
				for _, a := range cc.Assets() {
					ag := filepath.ToSlash(filepath.Join(base, a.Source))
					am, err := p.SourceFS.Glob(ag)
					if err != nil {
						continue
					}
					for _, rel := range am {
						spec := EntrySpec{
							Type:       EntryTypeAsset,
							SourceRoot: SourceTypeSource,
							Rel:        rel,
							BaseDir:    filepath.Dir(rel),
							ToTpl:      a.To,
							Pipeline:   a.Pipe,
						}
						if _, changed := plan.UpsertEntry(spec); changed {
							touched++
						}
					}
				}
			}

			// Layout-level assets (scheme)
			for _, a := range lyt.Assets() {
				sg := filepath.ToSlash(filepath.Join(base, a.Source))
				sm, err := p.SchemeFS.Glob(sg)
				if err != nil {
					continue
				}
				for _, rel := range sm {
					spec := EntrySpec{
						Type:       EntryTypeAsset,
						SourceRoot: SourceTypeScheme,
						Rel:        rel,
						BaseDir:    filepath.Dir(rel),
						Pipeline:   a.Pipe,
						ToTpl:      a.To,
					}
					if _, changed := plan.UpsertEntry(spec); changed {
						touched++
					}
				}
			}
		}
		return nil
	})

	return touched, err
}

func (p *Plan) UpsertEntry(spec EntrySpec) (*Entry, bool) {
	id := spec.Rel
	if spec.Type == EntryTypeAsset {
		id = fmt.Sprintf("%s:%s:%s", spec.Type, spec.SourceRoot, spec.Rel)
	}

	if entry, ok := p.Entry(id); ok {
		structChanged := false

		if entry.Type != spec.Type ||
			entry.SourceRoot != spec.SourceRoot ||
			entry.SourcePath != spec.Rel ||
			entry.BaseDir != spec.BaseDir ||
			entry.Pipeline != spec.Pipeline ||
			entry.ToTpl != spec.ToTpl ||
			(entry.Type == EntryTypeContent && entry.LayoutKey != spec.LayoutKey) {
			structChanged = true
		}

		entry.Type = spec.Type
		entry.SourceRoot = spec.SourceRoot
		entry.SourcePath = spec.Rel
		entry.BaseDir = spec.BaseDir
		entry.Pipeline = spec.Pipeline
		entry.TagName = spec.TagName
		entry.Layout = spec.Layout
		entry.LayoutKey = spec.LayoutKey
		entry.ToTpl = spec.ToTpl
		// entry.OutputPath = p.ComputeOutputPath(spec.BaseDir, spec.Rel, spec.Pipeline)

		if structChanged {
			entry.Dirty = true
		}
		return entry, structChanged
	}

	entry := &Entry{
		ID:         id,
		Type:       spec.Type,
		SourcePath: spec.Rel,
		SourceRoot: spec.SourceRoot,
		// TODO compute output using To attribute
		// OutputPath:     p.ComputeOutputPath(spec.BaseDir, spec.Rel, spec.Pipeline),
		ToTpl:    spec.ToTpl,
		Pipeline: spec.Pipeline,
		TagName:  spec.TagName,
		BaseDir:  spec.BaseDir,
	}

	if spec.Type == "content" && spec.Layout != nil {
		key := LayoutKey{RelLayoutPath: spec.Layout.Path(), BaseDir: spec.BaseDir}

		entry.Layout = spec.Layout
		entry.LayoutKey = key
	}

	entry.Dirty = true
	p.AddEntry(entry.ID, entry)
	return entry, true
}

func (p *Project) HashDirty(ctx context.Context, plan *Plan) (int, error) {
	entries := plan.GetAllEntries()

	type job struct{ e *Entry }
	jobs := make(chan job, len(entries))

	// TODO use config
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	var hashed int64

	// TODO use err chanel
	var mu sync.Mutex
	var errs []error

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if ctx.Err() != nil {
				mu.Lock()
				errs = append(errs, ctx.Err())
				mu.Unlock()
				continue
			}

			e := j.e

			if e.Type == EntryTypeContent && e.Layout != nil {
				if _, err := p.Layouts.GetOrBuild(ctx, p.SchemeFS, e.LayoutKey, e.Layout); err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("layout warmup %s: %w", e.ID, err))
					mu.Unlock()
					continue
				}
			}

			if err := p.updateEntryHashes(ctx, e, plan); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("update hashes %s: %w", e.ID, err))
				mu.Unlock()
				continue
			}

			// fmt.Printf("Hashed dirty: %t, %s: %s\n", e.Dirty, e.ID, e.Hash)
			// e.Dirty = true
			// if m, ok := plan.Manifest.Get(e.ID); !ok ||
			// 	m.Hash != e.Hash ||
			// 	m.OutputPath != e.OutputPath {
			// 	e.Dirty = true
			// } else {
			// 	e.Dirty = false
			// }

			atomic.AddInt64(&hashed, 1)
		}
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}
	for _, e := range entries {
		jobs <- job{e}
	}
	close(jobs)
	wg.Wait()

	return int(hashed), errors.Join(errs...)
}

// scanFiles walks the source directory for content
// func (p *Project) scanFiles(plan *Plan) (int, error) {
// 	srcFS := p.SourceRoot.FS()
//
// 	changed := 0
// 	err := fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
// 		if walkErr != nil {
// 			return walkErr
// 		}
//
// 		if d.IsDir() {
// 			if p.IgnoreFile(path) {
// 				return fs.SkipDir
// 			}
//
// 			base := filepath.ToSlash(path)
// 			lmap, err := p.GetLayouts(base)
// 			if err != nil {
// 				return nil
// 			}
//
// 			for _, lyt := range lmap {
// 				for _, cc := range lyt.Contents() {
// 					glob := filepath.ToSlash(filepath.Join(base, cc.Source))
// 					matches, err := fs.Glob(p.SourceRoot.FS(), glob)
// 					if err != nil {
// 						continue
// 					}
//
// 					for _, sourcePath := range matches {
// 						entry := p.createEntry("content", "source", sourcePath, cc.To, base, cc.Pipe, cc.Name, lyt)
//
// 						plan.AddEntry(entry.ID, entry)
// 						changed++
// 					}
//
// 					for _, a := range cc.Assets() {
// 						glob := filepath.ToSlash(filepath.Join(base, a.Source))
// 						matches, err := fs.Glob(p.SourceRoot.FS(), glob)
// 						if err != nil {
// 							continue
// 						}
//
// 						for _, sourcePath := range matches {
// 							entry := p.createEntry("asset", "source", sourcePath, a.To, filepath.Dir(sourcePath), a.Pipe, a.Name, nil)
//
// 							plan.AddEntry(entry.ID, entry)
// 							changed++
// 						}
// 					}
//
// 				}
//
// 				for _, a := range lyt.Assets() {
// 					glob := filepath.ToSlash(filepath.Join(base, a.Source))
// 					matches, err := fs.Glob(p.SchemeRoot.FS(), glob)
// 					if err != nil {
// 						continue
// 					}
//
// 					for _, sourcePath := range matches {
// 						entry := p.createEntry("asset", "scheme", sourcePath, a.To, filepath.Dir(sourcePath), a.Pipe, a.Name, nil)
//
// 						plan.AddEntry(entry.ID, entry)
//
// 						changed++
// 					}
// 				}
//
// 			}
// 		}
// 		return nil
// 	})
//
// 	return changed, err
// }

// PartialScan walks a directory tree and updates plan entries
// Stops at subdirectories with their own layouts.
// func (p *Project) PartialScan(ctx context.Context, plan *Plan, baseDir string) error {
// srcFS := p.SourceRoot.FS()

// log.DDebug(log.Plan).
// 	Str("base", baseDir).
// 	Msg("partial scan starting")
//
// startDir := baseDir
//
// return fs.WalkDir(srcFS, startDir, func(path string, d fs.DirEntry, walkErr error) error {
// 	if walkErr != nil {
// 		return walkErr
// 	}
//
// 	if ctx.Err() != nil {
// 		return ctx.Err()
// 	}
//
// 	if d.IsDir() {
// 		if p.IgnoreFile(path) {
// 			return fs.SkipDir
// 		}
//
// 		base := filepath.ToSlash(path)
// 		lmap, err := p.getLayoutsForDir(path)
// 		if base != startDir {
// 			if err == nil && len(lmap) > 0 {
// 				return fs.SkipDir
// 			}
// 		}
//
// 		for _, lyt := range lmap {
// 			key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}
//
// 			for _, cc := range lyt.Contents() {
// 				glob := filepath.ToSlash(filepath.Join(base, cc.Source))
// 				matches, err := fs.Glob(p.SourceRoot.FS(), glob)
// 				if err != nil {
// 					continue
// 				}
//
// 				for _, sourcePath := range matches {
// 					if err := p.updateOrCreateContentEntry(ctx, plan, sourcePath, lyt, key, cc); err != nil {
// 						log.Warn(fmt.Sprintf("updating or creating content %s: %v", sourcePath, err))
// 						continue
// 					}
// 				}
//
// 				for _, a := range cc.Assets() {
// 					glob := filepath.ToSlash(filepath.Join(base, a.Source))
// 					matches, err := fs.Glob(p.SourceRoot.FS(), glob)
// 					if err != nil {
// 						continue
// 					}
//
// 					for _, sourcePath := range matches {
// 						id := fmt.Sprintf("asset:source:%s", sourcePath)
// 						if err := p.updateOrCreateAssetEntry(ctx, plan, id, sourcePath, "source", a.Pipe, filepath.Dir(sourcePath), a); err != nil {
// 							log.Warn(fmt.Sprintf("updating or creating asset %s: %v", sourcePath, err))
// 							continue
//
// 						}
// 					}
// 				}
// 			}
//
// 			for _, a := range lyt.Assets() {
// 				glob := filepath.ToSlash(filepath.Join(base, a.Source))
// 				matches, err := fs.Glob(p.SchemeRoot.FS(), glob)
// 				if err != nil {
// 					continue
// 				}
//
// 				for _, sourcePath := range matches {
// 					id := fmt.Sprintf("asset:scheme:%s", sourcePath)
// 					if err := p.updateOrCreateAssetEntry(ctx, plan, id, sourcePath, "scheme", a.Pipe, filepath.Dir(sourcePath), a); err != nil {
// 						log.Warn(fmt.Sprintf("updating or creating asset %s: %v", sourcePath, err))
// 					}
// 				}
// 			}
// 		}
// 	}
//
// 	return nil
// })
// }

// computeHashes calculates hashes for all entries
// func (p *Plan) computeHashes(ctx context.Context, project *Project) error {
// 	var wg sync.WaitGroup
// 	sem := make(chan struct{}, 10)
// 	errChan := make(chan error, len(p.entries))
//
// 	for _, entry := range p.entries {
// 		if ctx.Err() != nil {
// 			break
// 		}
// 		wg.Add(1)
// 		go func(e *BuildEntry) {
// 			defer wg.Done()
//
// 			sem <- struct{}{}
// 			defer func() { <-sem }()
// 			if ctx.Err() != nil {
// 				return
// 			}
//
// 			if err := project.computeEntryHash(ctx, e, p); err != nil {
// 				errChan <- err
// 			}
// 		}(entry)
// 	}
//
// 	wg.Wait()
// 	close(errChan)
//
// 	var errs []error
// 	for err := range errChan {
// 		errs = append(errs, err)
// 	}
//
// 	return errors.Join(errs...)
// }

func (p *Project) computeEntryHash(ctx context.Context, e *Entry, plan *Plan) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var contentHash string
	var err error
	switch e.SourceRoot {
	case "source":
		contentHash, err = plan.project.SourceFS.HashFile(e.SourcePath)
	case "scheme":
		contentHash, err = plan.project.SchemeFS.HashFile(e.SourcePath)
	default:
		e.Error = fmt.Errorf("unknown source root: %s", e.SourceRoot)
		return nil
	}

	if err != nil {
		e.Error = fmt.Errorf("hash source: %w", err)
		return e.Error
	}
	e.ContentHash = contentHash

	// For content entries, get layout and fragment hashes from cache
	if e.Type == "content" && e.Layout != nil {
		layoutPath := e.Layout.Path()
		layoutHash, err := plan.project.SchemeFS.HashFile(layoutPath)
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
			fragHash, err := plan.project.SchemeFS.HashFile(frag)
			if err != nil {
				e.Error = fmt.Errorf("hash fragment %s: %w", frag, err)
				return e.Error
			}
			e.FragmentHashes = append(e.FragmentHashes, fragHash)
		}
	}

	if pl, err := p.Pipes.Get(e.Pipeline); err == nil {
		cfg := pl.Config()
		e.PipelineHash = utils.HashString(fmt.Sprintf("%s-%s-%s",
			cfg.PipelineName, cfg.SourceSuffix, cfg.TargetSuffix))
	}

	e.Hash = utils.CombineHashes(
		e.ContentHash,
		e.LayoutHash,
		e.PipelineHash,
		utils.CombineHashes(e.FragmentHashes...),
	)

	return nil
}

// updateEntryHashes recomputes all hashes for an entry
func (p *Project) updateEntryHashes(ctx context.Context, entry *Entry, plan *Plan) error {
	oldHash := entry.Hash

	if err := p.computeEntryHash(ctx, entry, plan); err != nil {
		return err
	}

	if oldHash != "" && entry.Hash != oldHash {
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
		entry.Dirty = false
		if p.project.Manifest.IsDirty(id, entry.outputPath, entry.Hash) {
			entry.Dirty = true
			changed++
		}
	}

	return changed
}

func (p *Plan) GetAllEntries() []*Entry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var all []*Entry
	for _, entry := range p.entries {
		all = append(all, entry)
	}
	return all
}

// GetDirtyEntries returns all entries that need rebuilding
// TODO could be removed
func (p *Plan) GetDirtyEntries() []*Entry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var dirty []*Entry
	for _, entry := range p.entries {
		if entry.Dirty {
			dirty = append(dirty, entry)
		}
	}
	return dirty
}

// ComputeOutputPath determines the output path for a file
// func (p *Project) ComputeOutputPath(base, rel, pipe string) string {
// 	pl, err := p.Pipes.Get(pipe)
// 	if err != nil {
// 		return filepath.Join(base, filepath.Base(rel))
// 	}
//
// 	cfg := pl.Config()
// 	target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(rel), "."+cfg.TargetSuffix)
// 	return filepath.Join(base, target)
// }

// func (p *Project) updateOrCreateContentEntry(
// 	ctx context.Context,
// 	plan *Plan,
// 	rel string,
// 	lyt *layout.Layout,
// 	key LayoutKey,
// 	cc layout.Content,
// ) error {
//
// 	entry, exists := plan.Entry(rel)
//
// 	if exists && entry.LayoutKey == key {
// 		err := plan.UpdateEntry(rel, func(e *BuildEntry) error {
// 			return p.updateEntryHashes(ctx, e, plan)
// 		})
//
// 		if err != nil {
// 			log.WWarn(log.Plan).
// 				Err(err).
// 				Str("id", rel).
// 				Msg("hash update failed, recreating entry")
// 		} else {
// 			return nil
// 		}
// 	}
//
// 	if exists {
// 		plan.DeleteEntry(rel)
// 	}
//
// 	entry, err := p.CreateEntryWithHash(ctx, plan, "content", "source", rel, cc.To, key.BaseDir, cc.Pipe, cc.Name, lyt)
// 	if err != nil {
// 		return err
// 	}
//
// 	entry.Dirty = true
// 	plan.AddEntry(entry.ID, entry)
//
// 	log.DDebug(log.Plan).
// 		Str("id", rel).
// 		Str("layout", key.RelLayoutPath).
// 		Msg("new content entry added")
//
// 	return nil
// }

// updateOrCreateAssetEntry updates an existing asset entry or creates a new one
// func (p *Project) updateOrCreateAssetEntry(
// 	ctx context.Context,
// 	plan *Plan,
// 	id string,
// 	rel string,
// 	sourceRoot SourceType,
// 	pipeline string,
// 	baseDir string,
// 	as layout.Asset,
// ) error {
// 	_, exists := plan.Entry(id)
//
// 	if exists {
// 		err := plan.UpdateEntry(id, func(e *BuildEntry) error {
// 			return p.updateEntryHashes(ctx, e, plan)
// 		})
//
// 		if err != nil {
// 			log.WWarn(log.Plan).
// 				Err(err).
// 				Str("id", id).
// 				Msg("hash update failed, recreating entry")
// 		} else {
// 			return nil
// 		}
// 	}
//
// 	entry, err := p.CreateEntryWithHash(ctx, plan, "asset", sourceRoot, rel, as.To, baseDir, pipeline, "", nil)
// 	if err != nil {
// 		return err
// 	}
//
// 	entry.Dirty = true
// 	plan.AddEntry(entry.ID, entry)
//
// 	log.DDebug(log.Plan).
// 		Str("id", id).
// 		Str("pipeline", pipeline).
// 		Msg("new asset entry added")
//
// 	return nil
// }

func (p *Plan) MarkDirtyByLayoutKeys(keys []LayoutKey) {
	if len(keys) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		for _, k := range keys {
			if e.LayoutKey == k {
				e.Dirty = true
			}
		}
	}
}
