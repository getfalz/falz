package project

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"craeft/internal/log"
	"craeft/internal/pipeline"
)

type Builder interface {
	Build(ctx context.Context, entries []*BuildEntry) (BuildStats, error)
}

type BuilderConfig struct {
	Concurrency  int  // Worker pool size (0 = NumCPU)
	UseCAS       bool // Use content-addressable storage cache
	ForceRebuild bool // Rebuild everything (ignore dirty flags)
	Variables    map[string]any
}

type builder struct {
	config *BuilderConfig

	fs       *FSRegistry
	layouts  LayoutRegistry
	pipes    *pipeline.Registry
	manifest *Manifest
	cas      *CAS
}

func NewBuilder(fs *FSRegistry, layouts LayoutRegistry, pipes *pipeline.Registry, manifest *Manifest, cas *CAS, config *BuilderConfig) Builder {
	if config.Concurrency <= 0 {
		config.Concurrency = runtime.NumCPU()
	}
	return &builder{
		pipes:   pipes,
		layouts: layouts,
		fs:      fs,

		cas:      cas,
		manifest: manifest,
		config:   config,
	}
}

// Build performs an incremental build
func (b *builder) Build(ctx context.Context, entries []*BuildEntry) (BuildStats, error) {
	if len(entries) == 0 {
		log.IInfo(log.Build).Msg("no changes detected, everything up to date")
		return BuildStats{}, nil
	}

	log.IInfo(log.Build).
		Int("total", len(entries)).
		Msg("starting incremental build")

	stats := b.runBuild(ctx, entries)
	stats.Total = len(entries)

	log.IInfo(log.Build).
		Int("total", stats.Total).
		Int64("built", stats.Built).
		Int64("cache_hits", stats.Cached).
		Int("errors", len(stats.Errors)).
		Msg("build completed")

	if len(stats.Errors) > 0 {
		return stats, errors.Join(stats.Errors...)
	}
	return stats, nil

}

// buildArtifact holds the result of processing an entry
type buildArtifact struct {
	Entry      *BuildEntry
	OutputPath string
	Data       []byte
	Cached     bool
	Error      error
}

func (b *builder) runBuild(
	ctx context.Context,
	entries []*BuildEntry,
) BuildStats {

	metrics := &buildMetrics{}

	buildChan := make(chan *BuildEntry, b.config.Concurrency*2)
	writeChan := make(chan *buildArtifact, b.config.Concurrency*4)
	errChan := make(chan error, len(entries))

	buildWg := b.buildStage(ctx, buildChan, writeChan, metrics, errChan)
	writeWg := b.writeStage(ctx, writeChan, metrics, errChan)

	func() {
		defer close(buildChan)

		for _, entry := range entries {
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
func (b *builder) buildStage(
	ctx context.Context,
	in <-chan *BuildEntry,
	out chan<- *buildArtifact,
	metrics *buildMetrics,
	errChan chan<- error,
) *sync.WaitGroup {

	var wg sync.WaitGroup
	sem := make(chan struct{}, b.config.Concurrency)

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

		for entry := range in {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(entry *BuildEntry) {
				defer wg.Done()
				defer func() { <-sem }()

				inCache := b.config.UseCAS && !b.config.ForceRebuild && b.cas.Has(entry.Hash)

				if inCache {
					artifact := &buildArtifact{
						Entry:  entry,
						Cached: true,
					}

					// compute output path
					lyt, err := b.layouts.Get(ctx, entry.LayoutKey)
					if err != nil {
						sendErr(artifact.Error)
						artifact.Error = fmt.Errorf("get layout %s: %w", entry.LayoutKey, err)
						return
					}

					outputPath, err := lyt.ContentOutput(entry.TagName, entry)
					if err != nil {
						artifact.Error = fmt.Errorf("compute output path: %w", err)
						return
					}

					artifact.OutputPath = outputPath

					metrics.cached.Add(1)
					metrics.built.Add(1)

					select {
					case <-ctx.Done():
						return
					case out <- artifact:
					}
					return
				}

				artifact := b.buildEntry(ctx, entry)
				metrics.built.Add(1)

				if artifact.Error != nil {
					sendErr(artifact.Error)
					return
				}

				// store in cache
				if b.config.UseCAS && artifact.Data != nil {
					if err := b.cas.Store(entry.Hash, artifact.Data); err != nil {
						log.WWarn(log.Build).Err(err).Msgf("failed to store in CAS for entry %s", entry.ID)
					}
				}

				select {
				case out <- artifact:
				case <-ctx.Done():
					return
				}
			}(entry)
		}
	}()

	return &wg
}

// @ts type global.tCtx
// @doc Context passed into the `transform` function.
// @field vars: Record<string, unknown> - Variables set in the config.yaml file. Custom key/value mappings loaded from `config.yaml`. Values may include environment‑variable references (e.g. `$HOME/docs`), which will be expanded at runtime.
func (b *builder) buildEntry(
	ctx context.Context,
	entry *BuildEntry,
) *buildArtifact {
	start := time.Now()
	artifact := &buildArtifact{Entry: entry}

	data, err := b.fs.Get(entry.SourceRoot).ReadFile(entry.SourcePath)
	if err != nil {
		artifact.Error = fmt.Errorf("read %s: %w", entry.SourcePath, err)
		return artifact
	}

	pl, err := b.pipes.Get(entry.Pipeline)
	if err != nil {
		artifact.Error = fmt.Errorf("get pipeline %s: %w", entry.Pipeline, err)
		return artifact
	}

	lyt, err := b.layouts.Get(ctx, entry.LayoutKey)
	if err != nil {
		artifact.Error = fmt.Errorf("get layout %s: %w", entry.LayoutKey, err)
		return artifact
	}

	outputPath, err := lyt.ContentOutput(entry.TagName, entry)
	if err != nil {
		artifact.Error = fmt.Errorf("compute output path: %w", err)
		return artifact
	}

	artifact.OutputPath = outputPath

	vars := pipeline.Vars{
		"target": outputPath,
		"vars":   b.config.Variables,
	}

	transformed, err := pl.Transform(ctx, data, vars)
	if err != nil {
		artifact.Error = fmt.Errorf("transform %s: %w", entry.SourcePath, err)
		return artifact
	}

	// For content entries, apply layout template
	if entry.Type == ContentType && lyt != nil {
		vars.Set(entry.TagName, transformed)

		output := lyt.ExecuteString(vars)
		artifact.Data = []byte(output)
	} else {
		artifact.Data = []byte(transformed)
	}

	log.DDebug(log.Build).
		Str("source", entry.SourcePath).
		Str("output", outputPath).
		Str("pipe", entry.Pipeline).
		Int("bytes", len(artifact.Data)).
		Dur("duration", time.Since(start)).
		Msg("built entry")

	return artifact
}

// writeStage batches the artifacts gotten in the channel in. Whenever the batch reach a size of 10, or 100 ms are passed, artifacts are flushed to the file system
func (b *builder) writeStage(
	ctx context.Context,
	in <-chan *buildArtifact,
	metrics *buildMetrics,
	errChan chan<- error,
) *sync.WaitGroup {

	// TODO check this value
	numWorkers := min(4, max(2, b.config.Concurrency/4))
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
				for _, artifacts := range dirBatch {
					for _, artifact := range artifacts {
						if err := b.writeArtifact(artifact); err != nil {
							sendErr(fmt.Errorf("write result %s: %w", artifact.Entry.ID, err))
							continue
						}

						// outputPath is already computed in writeArtifact
						b.manifest.Set(artifact.Entry.ID, ManifestEntry{
							Hash:       artifact.Entry.Hash,
							OutputPath: artifact.OutputPath,
						})

						metrics.written.Add(1)
					}
				}
				clear(dirBatch)
			}

			for {
				select {
				case <-ctx.Done():
					flush()
					return

				case artifact, ok := <-in:
					if !ok {
						flush()
						return
					}

					dir := filepath.Dir(artifact.OutputPath)
					dirBatch[dir] = append(dirBatch[dir], artifact)

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

// writeArtifact writes the processed artifact to the file system
func (b *builder) writeArtifact(
	artifact *buildArtifact,
) error {
	entry := artifact.Entry

	if err := b.fs.Output().MkdirAll(filepath.Dir(artifact.OutputPath), 0755); err != nil {
		return fmt.Errorf("create dir for %s: %w", artifact.OutputPath, err)
	}

	if artifact.Cached {
		if err := b.cas.Link(entry.Hash, artifact.OutputPath); err != nil {
			return fmt.Errorf("restore from CAS %s: %w", artifact.OutputPath, err)
		}
	} else {
		b.fs.Output().WriteFile(artifact.OutputPath, artifact.Data, 0644)
	}

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
