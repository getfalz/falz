package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"craeft/internal/log"
	"craeft/internal/pipeline"
)

// Build performs an incremental build
func (p *Project) Build(ctx context.Context) (RunStats, error) {
	stats := RunStats{}

	planStart := time.Now()
	plan, err := p.Plan(ctx)
	if err != nil {
		return stats, fmt.Errorf("generate plan: %w", err)
	}

	stats.PlanDuration = time.Since(planStart)

	buildStart := time.Now()
	stats.BuildStats, err = p.BuildFromPlan(ctx, plan)
	stats.BuildDuration = time.Since(buildStart)

	return stats, err
}

// BuildFromPlan builds only dirty entries from an existing plan
func (p *Project) BuildFromPlan(ctx context.Context, plan *Plan) (BuildStats, error) {
	stats := BuildStats{
		TotalEntries: len(plan.entries),
	}

	if p.Config.Build.CleanStale {
		removed := p.cleanStaleEntries(plan)
		if len(removed) > 0 {
			log.IInfo(log.Build).Int("removed", len(removed)).Msg("cleaned stale entries")
		}
	}

	dirtyEntries := plan.GetDirtyEntries()
	if p.Config.Build.ForceRebuild {
		dirtyEntries = plan.GetAllEntries()
	}
	stats.DirtyEntries = len(dirtyEntries)

	if len(dirtyEntries) == 0 {
		log.IInfo(log.Build).Msg("no changes detected, everything up to date")
		return stats, nil
	}

	log.IInfo(log.Build).
		Int("total", stats.TotalEntries).
		Int("dirty", stats.DirtyEntries).
		Msg("starting incremental build")

	stats.BuildSummary = p.runBuild(ctx, plan, dirtyEntries)

	if err := plan.Manifest.Save(filepath.Join(p.Config.OutputPath, manifestoFile)); err != nil {
		log.Warn(fmt.Sprintf("failed to save manifest: %v", err))
	}

	log.IInfo(log.Build).
		Int("total", stats.TotalEntries).
		Int("dirty", stats.DirtyEntries).
		Int64("built", stats.Built).
		Int64("cache_hits", stats.CacheHits).
		Int64("cache_misses", stats.CacheMisses).
		Int("errors", len(stats.Errors)).
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

		if err := p.outputFS.Remove(entry.OutputPath); err != nil && !os.IsNotExist(err) {
			log.Warn(fmt.Sprintf("failed to remove stale output %s: %v", entry.OutputPath, err))
		}
	}

	return removedIds
}

// buildArtifact holds the result of processing an entry
type buildArtifact struct {
	Entry  *Entry
	Data   []byte
	Error  error
	Cached bool
}


func (p *Project) runBuild(
	ctx context.Context,
	plan *Plan,
	entries []*Entry,
) BuildSummary {

	metrics := &buildMetrics{}

	buildChan := make(chan *Entry, p.Config.Build.Concurrency*2)
	writeChan := make(chan *buildArtifact, p.Config.Build.Concurrency*4)
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
	var errors []error
	for err := range errChan {

		log.EError(log.Build).Err(err).Msg("entry build error")

		errors = append(errors, err)
	}

	if ctx.Err() != nil {
		errors = append(errors, ctx.Err())
	}

	result := metrics.Snapshot()
	result.Errors = errors

	return result
}

// buildStage builds all Entries it gets in the in channel, and write the output artifacts into the out channel. Any error is written to the errChan channel. It will update the given metrics
// TODO can we remove the plan dependency?
func (p *Project) buildStage(
	ctx context.Context,
	in <-chan *Entry,
	out chan<- *buildArtifact,
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
			go func(entry *Entry) {
				defer wg.Done()
				defer func() { <-sem }()

				if entry.Error != nil {
					sendErr(fmt.Errorf("planning error for %s: %w", entry.ID, entry.Error))
					// log.Warn(fmt.Sprintf("skipping entry with planning error: %s: %v", w.ID, w.Error))
					metrics.processed.Add(1)
					return
				}

				var result *buildArtifact
				inCache := p.Config.Build.UseCAS && !p.Config.Build.ForceRebuild && plan.CAS != nil && plan.CAS.Has(entry.Hash)

				if inCache {
					result = &buildArtifact{
						Entry:  entry,
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

				result = p.buildEntry(ctx, entry)

				if result.Error == nil {
					metrics.cacheMisses.Add(1)

					if p.Config.Build.UseCAS && plan.CAS != nil && result.Data != nil {
						if err := plan.CAS.Store(entry.Hash, result.Data); err == nil {
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

// writeStage batches the artifacts gotten in the channel in. Whenever the batch reach a size of 10, or 100 ms are passed, artifacts are flushed to the file system
func (p *Project) writeStage(
	ctx context.Context,
	in <-chan *buildArtifact,
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

			dirBatch := make(map[string][]*buildArtifact)
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
						if err := p.writeArtifacts(plan, r); err != nil {
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
func (p *Project) buildEntry(
	ctx context.Context,
	entry *Entry,
) *buildArtifact {
	start := time.Now()
	result := &buildArtifact{Entry: entry}

	var data []byte
	var err error
	switch entry.SourceRoot {
	case "source":
		data, err = p.SourceFS.ReadFile(filepath.ToSlash(entry.SourcePath))
	case "scheme":
		data, err = p.SchemeFS.ReadFile(filepath.ToSlash(entry.SourcePath))
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

// writeArtifacts writes the processed artifact to the file system
func (p *Project) writeArtifacts(
	plan *Plan,
	artifact *buildArtifact,
) error {
	entry := artifact.Entry

	outputPath, err := entry.OutputPath()
	if err != nil {
		return fmt.Errorf("compute output path: %w", err)
	}
	fullPath := filepath.Join(p.Config.OutputPath, outputPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("create dir for %s: %w", outputPath, err)
	}

	if artifact.Cached {
		if err := plan.CAS.Link(entry.Hash, fullPath); err != nil {
			return fmt.Errorf("restore from CAS %s: %w", outputPath, err)
		}
	} else {
		tmpFile := fullPath + ".tmp"

		if err := os.WriteFile(tmpFile, artifact.Data, 0644); err != nil {
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

	plan.UpdateEntry(artifact.Entry.ID, func(e *Entry) error {
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
