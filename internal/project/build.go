package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"craeft/internal/log"
	"craeft/internal/pipeline"
)

// BuildOptions configures the incremental build
type BuildOptions struct {
	ForceRebuild bool // Ignore cache, rebuild everything
	Concurrency  int  // Number of concurrent workers
	UseCAS       bool // Use content-addressable storage
	CleanStale   bool // Remove outputs for deleted sources
}

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
func (p *Project) Build(ctx context.Context, opts BuildOptions) (*BuildStats, error) {
	start := time.Now()

	planStart := time.Now()
	plan, err := p.Plan(ctx)
	if err != nil {
		return &BuildStats{}, fmt.Errorf("generate plan: %w", err)
	}

	p.currentPlanMu.Lock()
	p.currentPlan = plan
	p.currentPlanMu.Unlock()

	planDuration := time.Since(planStart)

	stats, err := p.BuildFromPlan(ctx, plan, opts)
	stats.PlanDuration = planDuration
	stats.Duration = time.Since(start)

	return stats, err
}

// BuildFromPlan builds only dirty entries from an existing plan
func (p *Project) BuildFromPlan(ctx context.Context, plan *Plan, opts BuildOptions) (*BuildStats, error) {
	start := time.Now()
	stats := &BuildStats{
		TotalEntries: len(plan.entries),
	}

	if opts.Concurrency <= 0 {
		opts.Concurrency = runtime.NumCPU()
	}

	if opts.CleanStale {
		removed := p.cleanStaleEntries(plan)
		if len(removed) > 0 {
			log.IInfo(log.Build).Int("removed", len(removed)).Msg("cleaned stale entries")
		}
	}

	dirtyEntries := plan.GetDirtyEntries()
	stats.DirtyEntries = len(dirtyEntries)

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
	buildStats := p.runBuild(ctx, plan, dirtyEntries, opts)
	stats.BuildDuration = time.Since(buildStart)

	stats.BuiltEntries = buildStats.Built
	stats.CacheHits = buildStats.CacheHits
	stats.CacheMisses = buildStats.CacheMisses
	stats.BytesWritten = buildStats.BytesWritten
	stats.BytesCached = buildStats.BytesCached
	stats.Errors = buildStats.Errors

	// TODO change manifest path
	if err := plan.Manifest.Save(filepath.Join(p.Config.OutputPath, ".craeft-manifest.json")); err != nil {
		log.Warn(fmt.Sprintf("failed to save manifest: %v", err))
	}

	stats.Duration = time.Since(start)

	log.IInfo(log.Build).
		Int("total", stats.TotalEntries).
		Int("dirty", stats.DirtyEntries).
		Int64("built", stats.BuiltEntries).
		Int64("cache_hits", stats.CacheHits).
		Int64("cache_misses", stats.CacheMisses).
		Dur("duration", stats.Duration).
		Msg(" build completed")

	return stats, errors.Join(stats.Errors...)
}

// cleanStaleEntries removes outputs for deleted source files
func (p *Project) cleanStaleEntries(plan *Plan) []string {
	currentIDs := make(map[string]struct{})
	for id := range plan.entries {
		currentIDs[id] = struct{}{}
	}

	removed := plan.Manifest.CleanStale(currentIDs)

	for _, id := range removed {
		if entry, ok := plan.Manifest.Get(id); ok {
			if err := p.outRoot.Remove(entry.OutputPath); err != nil && !os.IsNotExist(err) {
				log.Warn(fmt.Sprintf("failed to remove stale output %s: %v", entry.OutputPath, err))
			}
		}
	}

	return removed
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
	opts BuildOptions,
) *BuildResult {
	result := &BuildResult{
		Errors: make([]error, 0),
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	metrics := &buildMetrics{}

	buildChan := make(chan *BuildEntry, 100)
	writeChan := make(chan *ProcessResult, 100)
	errChan := make(chan error, len(entries))

	buildWg := p.buildStage(ctx, buildChan, writeChan, plan, opts, metrics, errChan)

	writeWg := p.writeStage(ctx, writeChan, plan, opts, metrics, errChan)

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
		result.Errors = append(result.Errors, err)
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
	opts BuildOptions,
	metrics *buildMetrics,
	errChan chan<- error,
) *sync.WaitGroup {

	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.Concurrency)

	wg.Add(1)
	go func() {
		defer wg.Done()

		for work := range in {
			if ctx.Err() != nil {
				return
			}

			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(w *BuildEntry) {
				defer wg.Done()
				defer func() { <-sem }()

				if ctx.Err() != nil {
					return
				}

				if work.Error != nil {
					log.Warn(fmt.Sprintf("skipping entry with planning error: %s: %v", work.ID, work.Error))
					metrics.processed.Add(1)
					return
				}

				var result *ProcessResult
				inCache := opts.UseCAS && !opts.ForceRebuild && plan.CAS != nil && plan.CAS.Has(work.Hash)

				if inCache {
					var data []byte
					data, err := plan.CAS.Restore(work.Hash)
					if err == nil {
						result = &ProcessResult{
							Entry:  work,
							Data:   data,
							Cached: true,
						}
						metrics.cacheHits.Add(1)
					} else {
						// TODO warn?
						log.Warn(fmt.Sprintf("cache restore failed for %s: %v", work.ID, err))
						result = &ProcessResult{
							Entry:  work,
							Error:  fmt.Errorf("cache restore failed: %w", err),
							Cached: false,
						}
					}
				}

				if !inCache || result.Error != nil {
					result = p.processEntry(ctx, work)

					if result.Error == nil {
						metrics.cacheMisses.Add(1)

						if opts.UseCAS && plan.CAS != nil && result.Data != nil {
							if err := plan.CAS.Store(work.Hash, result.Data); err != nil {
								// TODO warn?
								log.Warn(fmt.Sprintf("cache store failed for %s: %v", work.ID, err))
							} else {
								metrics.bytesCached.Add(int64(len(result.Data)))
							}
						}
					}
				}

				metrics.processed.Add(1)

				if result.Error != nil {
					if ctx.Err() != nil {
						return
					}
					select {
					case <-ctx.Done():
						return
					case errChan <- result.Error:
					default:
						log.EError(log.Build).
							Err(result.Error).
							Msg("BUG: error channel full despite being sized to len(entries)")
					}
				}

				// cached results with errors should not be written
				if !result.Cached && result.Error == nil && result.Data != nil {
					if ctx.Err() != nil {
						return
					}
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
	opts BuildOptions,
	metrics *buildMetrics,
	errChan chan<- error,
) *sync.WaitGroup {

	numWorkers := min(4, max(2, opts.Concurrency/4))
	var wg sync.WaitGroup

	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			dirBatch := make(map[string][]*ProcessResult)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			flush := func() {
				for _, results := range dirBatch {
					for _, r := range results {
						if err := p.writeResult(plan, r); err != nil {
							log.Warn(fmt.Sprintf("write failed for %s: %v", r.Entry.OutputPath, err))

							if ctx.Err() != nil {
								return
							}
							select {
							case <-ctx.Done():
								return
							case errChan <- err:
							default:
								log.EError(log.Build).
									Err(err).
									Msg("BUG: error channel full despite being sized to len(entries)")
							}
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

					dir := filepath.Dir(result.Entry.OutputPath)
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
	lb := log.TTrace(log.Build).
		Str("source", entry.SourcePath).
		Str("pipe", entry.Pipeline)
	if entry.Type == "content" && entry.Layout != nil {
		lb.Str("layout", entry.Layout.Path())
	}
	lb.Msg("build start")

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

	vars := pipeline.Vars{
		"target": entry.OutputPath,
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

		vars.Set(entry.ContentName, transformed)
		output := tpl.ExecuteString(vars)
		result.Data = []byte(output)
	} else {
		result.Data = []byte(transformed)
	}

	lb = log.DDebug(log.Build).
		Str("source", entry.SourcePath).
		Str("pipe", entry.Pipeline).
		Dur("build_ms", time.Since(start))
	if entry.Type == "content" && entry.Layout != nil {
		lb.Str("layout", entry.Layout.Path())
	}
	lb.Msg("build done")

	return result
}

// writeResult writes the processed result to output
func (p *Project) writeResult(
	plan *Plan,
	result *ProcessResult,
) error {
	entry := result.Entry

	dst := filepath.Join(entry.BaseDir, entry.OutputPath)
	fullPath := filepath.Join(p.Config.OutputPath, dst)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("create dir for %s: %w", dst, err)
	}

	if err := os.WriteFile(fullPath, result.Data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}

	log.TTrace(log.IO).
		Str("dst", dst).
		Int("bytes", len(result.Data)).
		Msg("wrote file")

	plan.Manifest.Set(entry.ID, ManifestEntry{
		Hash:       entry.Hash,
		OutputPath: entry.OutputPath,
		Built:      time.Now(),
		Size:       int64(len(result.Data)),
	})

	plan.UpdateEntry(result.Entry.ID, func(e *BuildEntry) error {
		e.Dirty = false
		return nil
	})

	log.TTrace(log.Build).
		Str("id", entry.ID).
		Str("type", entry.Type).
		Str("output", entry.OutputPath).
		Bool("cached", result.Cached).
		Msg("entry built")

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
