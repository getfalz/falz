package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"craeft/internal/log"
	"craeft/internal/pipeline"
)

// BuildStats tracks build statistics
type BuildStats struct {
	TotalEntries  int
	DirtyEntries  int
	BuiltEntries  int64
	CacheHits     int64
	CacheMisses   int64
	BytesWritten  int64
	BytesCached   int64
	Duration      time.Duration
	PlanDuration  time.Duration
	BuildDuration time.Duration
	Errors        []error
}

func (b *BuildStats) String() string {
	return fmt.Sprintf(
		"Total: %d, Dirty: %d, Built: %d, Cache Hits: %d, Cache Misses: %d, Bytes Written: %d, Bytes Cached: %d, Duration: %s (Plan: %s, Build: %s)",
		b.TotalEntries, b.DirtyEntries, b.BuiltEntries, b.CacheHits, b.CacheMisses, b.BytesWritten, b.BytesCached, b.Duration, b.PlanDuration, b.BuildDuration,
	)
}

// ProcessResult holds the result of processing an entry
type ProcessResult struct {
	Entry  *BuildEntry
	Data   []byte
	Error  error
	Cached bool
}

// Build performs an incremental build
func (p *Project) Build(ctx context.Context) (*BuildStats, error) {
	start := time.Now()

	planStart := time.Now()
	plan, err := p.Plan(ctx)
	if err != nil {
		return &BuildStats{}, fmt.Errorf("generate plan: %w", err)
	}

	planDuration := time.Since(planStart)

	stats, err := p.BuildFromPlan(ctx, plan)
	stats.PlanDuration = planDuration
	stats.Duration = time.Since(start)

	return stats, err
}

// BuildFromPlan builds only dirty entries from an existing plan
func (p *Project) BuildFromPlan(ctx context.Context, plan *Plan) (*BuildStats, error) {
	start := time.Now()
	stats := &BuildStats{
		TotalEntries: len(plan.entries),
	}

	if p.Config.Build.CleanStale {
		removed := p.cleanStaleEntries(plan)
		if len(removed) > 0 {
			log.IInfo(log.Build).Int("removed", len(removed)).Msg("cleaned stale entries")
		}
	}

	dirtyEntries := plan.GetDirtyEntries()
	stats.DirtyEntries = len(dirtyEntries)

	if p.Config.Build.ForceRebuild {
		dirtyEntries = plan.GetAllEntries()
	}

	if len(dirtyEntries) == 0 {
		log.IInfo(log.Build).Msg("no changes detected, everything up to date")
		stats.Duration = time.Since(start)
		return stats, nil
	}

	log.IInfo(log.Build).
		Int("total", stats.TotalEntries).
		Int("dirty", stats.DirtyEntries).
		Msg("starting incremental build")

	buildStart := time.Now()
	buildStats := p.runBuild(ctx, plan, dirtyEntries)
	stats.BuildDuration = time.Since(buildStart)

	stats.BuiltEntries = buildStats.Built
	stats.CacheHits = buildStats.CacheHits
	stats.CacheMisses = buildStats.CacheMisses
	stats.BytesWritten = buildStats.BytesWritten
	stats.BytesCached = buildStats.BytesCached
	stats.Errors = buildStats.Errors

	if err := plan.Manifest.Save(filepath.Join(p.Config.OutputPath, manifestoFile)); err != nil {
		log.Warn(fmt.Sprintf("failed to save manifest: %v", err))
	}

	stats.Duration = time.Since(start)

	log.IInfo(log.Build).
		Int("total", stats.TotalEntries).
		Int("dirty", stats.DirtyEntries).
		Int64("built", stats.BuiltEntries).
		Int64("cache_hits", stats.CacheHits).
		Int64("cache_misses", stats.CacheMisses).
		Int("errors", len(stats.Errors)).
		Dur("build", stats.BuildDuration).
		Dur("duration", stats.Duration).
		Msg("build completed")

	if len(stats.Errors) > 0 {
		return stats, &BuildError{Errors: stats.Errors}
	}
	return stats, nil
}

// cleanStaleEntries removes outputs for deleted source files
func (p *Project) cleanStaleEntries(plan *Plan) []string {
	currentIDs := make(map[string]struct{})
	for id := range plan.entries {
		currentIDs[id] = struct{}{}
	}

	removed := plan.Manifest.CleanStale(currentIDs)

	var removedIds []string
	for _, entry := range removed {
		removedIds = append(removedIds, entry.OutputPath)

		if err := p.outRoot.Remove(entry.OutputPath); err != nil && !os.IsNotExist(err) {
			log.Warn(fmt.Sprintf("failed to remove stale output %s: %v", entry.OutputPath, err))
		}
	}

	return removedIds
}

type BuildResult struct {
	Built        int64
	CacheHits    int64
	CacheMisses  int64
	BytesWritten int64
	BytesCached  int64
	Errors       []error
}

type buildMetrics struct {
	processed    atomic.Int64
	cached       atomic.Int64
	written      atomic.Int64
	cacheHits    atomic.Int64
	cacheMisses  atomic.Int64
	bytesWritten atomic.Int64
	bytesCached  atomic.Int64
}

func (p *Project) runBuild(
	ctx context.Context,
	plan *Plan,
	entries []*BuildEntry,
) *BuildResult {

	result := &BuildResult{
		Errors: make([]error, 0),
	}

	metrics := &buildMetrics{}

	buildChan := make(chan *BuildEntry, p.Config.Build.Concurrency*2)
	writeChan := make(chan *ProcessResult, p.Config.Build.Concurrency*4)
	errChan := make(chan error, len(entries))

	buildWg := p.buildStage(ctx, buildChan, writeChan, plan, metrics, errChan)
	writeWg := p.writeStage(ctx, writeChan, plan, metrics, errChan)

	func() {
		defer close(buildChan)

		for _, entry := range entries {
			if ctx.Err() != nil {
				return
			}
			select {
			case buildChan <- entry:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		buildWg.Wait()
		close(writeChan)
	}()

	writeWg.Wait()

	close(errChan)
	for err := range errChan {

		log.EError(log.Build).Err(err).Msg("entry build error")

		result.Errors = append(result.Errors, err)
	}

	if ctx.Err() != nil {
		result.Errors = append(result.Errors, ctx.Err())
	}

	result.Built = metrics.written.Load()
	result.CacheHits = metrics.cacheHits.Load()
	result.CacheMisses = metrics.cacheMisses.Load()
	result.BytesWritten = metrics.bytesWritten.Load()
	result.BytesCached = metrics.bytesCached.Load()

	return result
}

func (p *Project) buildStage(
	ctx context.Context,
	in <-chan *BuildEntry,
	out chan<- *ProcessResult,
	plan *Plan,
	metrics *buildMetrics,
	errChan chan<- error,
) *sync.WaitGroup {

	var wg sync.WaitGroup
	sem := make(chan struct{}, p.Config.Build.Concurrency)

	sendErr := func(err error) {
		select {
		case <-ctx.Done():
			return
		case errChan <- err:
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for work := range in {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(w *BuildEntry) {
				defer wg.Done()
				defer func() { <-sem }()

				if w.Error != nil {
					sendErr(fmt.Errorf("planning error for %s: %w", w.ID, w.Error))
					// log.Warn(fmt.Sprintf("skipping entry with planning error: %s: %v", w.ID, w.Error))
					metrics.processed.Add(1)
					return
				}

				var result *ProcessResult
				inCache := p.Config.Build.UseCAS && !p.Config.Build.ForceRebuild && plan.CAS != nil && plan.CAS.Has(w.Hash)

				if inCache {
					// outputPath, err := w.OutputPath()
					// if err != nil {
					// 	metrics.processed.Add(1)
					// 	sendErr(fmt.Errorf("compute output path for %s: %w", w.ID, err))
					// 	return
					// }
					//
					// foutputPath := filepath.Join(p.Config.OutputPath, outputPath)
					// if err = plan.CAS.Link(w.Hash, foutputPath); err == nil {
					// 	// TODO check if other fields are required
					// 	result = &ProcessResult{
					// 		Entry:  w,
					// 		Cached: true,
					// 	}
					// 	metrics.cacheHits.Add(1)
					// 	metrics.processed.Add(1)
					//
					// 	select {
					// 	case <-ctx.Done():
					// 		return
					// 	case out <- result:
					// 	}
					// 	return
					// }
					//
					// log.Warn(fmt.Sprintf("CAS restore failed for %s: %v", w.ID, err))

					result = &ProcessResult{
						Entry:  w,
						Cached: true,
					}
					metrics.cacheHits.Add(1)
					metrics.processed.Add(1)

					select {
					case <-ctx.Done():
						return
					case out <- result:
					}
					return
				}

				result = p.processEntry(ctx, w)

				if result.Error == nil {
					metrics.cacheMisses.Add(1)

					if p.Config.Build.UseCAS && plan.CAS != nil && result.Data != nil {
						if err := plan.CAS.Store(w.Hash, result.Data); err == nil {
							metrics.bytesCached.Add(int64(len(result.Data)))
						}
					}
				}

				metrics.processed.Add(1)

				if result.Error != nil {
					sendErr(result.Error)
					return
				}
				if result.Data != nil {
					select {
					case out <- result:
					case <-ctx.Done():
						return
					}
				}
			}(work)
		}
	}()

	return &wg
}

func (p *Project) writeStage(
	ctx context.Context,
	in <-chan *ProcessResult,
	plan *Plan,
	metrics *buildMetrics,
	errChan chan<- error,
) *sync.WaitGroup {

	numWorkers := min(4, max(2, p.Config.Build.Concurrency/4))
	var wg sync.WaitGroup

	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			dirBatch := make(map[string][]*ProcessResult)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			sendErr := func(err error) {
				select {
				case <-ctx.Done():
					return
				case errChan <- err:
				}
			}

			flush := func() {
				for _, results := range dirBatch {
					for _, r := range results {
						if err := p.writeResult(plan, r); err != nil {
							sendErr(fmt.Errorf("write result %s: %w", r.Entry.ID, err))
							continue
						}

						metrics.written.Add(1)
						if r.Data != nil {
							metrics.bytesWritten.Add(int64(len(r.Data)))
						}
					}
				}
				clear(dirBatch)
			}

			for {
				select {
				case <-ctx.Done():
					flush()
					return

				case result, ok := <-in:
					if !ok {
						flush()
						return
					}

					outputPath, err := result.Entry.OutputPath()
					if err != nil {
						sendErr(fmt.Errorf("compute output path for %s: %w", result.Entry.ID, err))
					}
					dir := filepath.Dir(outputPath)
					dirBatch[dir] = append(dirBatch[dir], result)

					if len(dirBatch[dir]) >= 10 {
						flush()
					}

				case <-ticker.C:
					if len(dirBatch) > 0 {
						flush()
					}
				}
			}
		}()
	}

	return &wg
}

// @ts type global.tCtx
// @doc Context passed into the `transform` function.
// @field vars: Record<string, unknown> - Variables set in the config.yaml file. Custom key/value mappings loaded from `config.yaml`. Values may include environment‑variable references (e.g. `$HOME/docs`), which will be expanded at runtime.
func (p *Project) processEntry(
	ctx context.Context,
	entry *BuildEntry,
) *ProcessResult {
	// lb := log.TTrace(log.Build).
	// 	Str("source", entry.SourcePath).
	// 	Str("pipe", entry.Pipeline)
	// if entry.Type == "content" && entry.Layout != nil {
	// 	lb.Str("layout", entry.Layout.Path())
	// }
	// lb.Msg("build start")

	start := time.Now()
	result := &ProcessResult{Entry: entry}

	var data []byte
	var err error
	switch entry.SourceRoot {
	case "source":
		data, err = p.readSource(entry.SourcePath)
	case "scheme":
		data, err = p.readScheme(entry.SourcePath)
	default:
		result.Error = fmt.Errorf("unknown source root: %s", entry.SourceRoot)
		return result
	}

	if err != nil {
		result.Error = fmt.Errorf("read %s: %w", entry.SourcePath, err)
		return result
	}

	pl, err := p.Pipes.Get(entry.Pipeline)
	if err != nil {
		result.Error = fmt.Errorf("get pipeline %s: %w", entry.Pipeline, err)
		return result
	}

	outputPath, err := entry.OutputPath()
	if err != nil {
		result.Error = fmt.Errorf("compute output path: %w", err)
		return result
	}
	vars := pipeline.Vars{
		"target": outputPath,
		"vars":   p.Config.Variables,
	}

	transformed, err := pl.Transform(ctx, data, vars)
	if err != nil {
		result.Error = fmt.Errorf("transform %s: %w", entry.SourcePath, err)
		return result
	}

	// For content entries, apply layout template
	if entry.Type == "content" && entry.Layout != nil {
		tpl, err := p.templateForLayout(ctx, entry.LayoutKey)
		if err != nil {
			result.Error = fmt.Errorf("get template: %w", err)
			return result
		}

		vars.Set(entry.TagName, transformed)
		output := tpl.ExecuteString(vars)
		result.Data = []byte(output)
	} else {
		result.Data = []byte(transformed)
	}

	log.DDebug(log.Build).
		Str("source", entry.SourcePath).
		Str("output", outputPath).
		Str("pipe", entry.Pipeline).
		Int("bytes", len(result.Data)).
		Dur("duration", time.Since(start)).
		Msg("built")

	return result
}

// writeResult writes the processed result to output
func (p *Project) writeResult(
	plan *Plan,
	result *ProcessResult,
) error {
	entry := result.Entry

	outputPath, err := entry.OutputPath()
	if err != nil {
		return fmt.Errorf("compute output path: %w", err)
	}
	fullPath := filepath.Join(p.Config.OutputPath, outputPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("create dir for %s: %w", outputPath, err)
	}

	if result.Cached {
		if err := plan.CAS.Link(entry.Hash, fullPath); err != nil {
			return fmt.Errorf("restore from CAS %s: %w", outputPath, err)
		}
	} else {
		tmpFile := fullPath + ".tmp"

		if err := os.WriteFile(tmpFile, result.Data, 0644); err != nil {
			return fmt.Errorf("write temp %s: %w", outputPath, err)
		}

		if f, err := os.OpenFile(tmpFile, os.O_RDWR, 0644); err == nil {
			_ = f.Sync()
			_ = f.Close()
		}

		if err := os.Rename(tmpFile, fullPath); err != nil {
			_ = os.Remove(tmpFile)
			return fmt.Errorf("rename %s: %w", outputPath, err)
		}
	}

	plan.Manifest.Set(entry.ID, ManifestEntry{
		Hash:       entry.Hash,
		OutputPath: outputPath,
	})

	plan.UpdateEntry(result.Entry.ID, func(e *BuildEntry) error {
		e.Dirty = false
		return nil
	})

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
