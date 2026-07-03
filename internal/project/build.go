package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"craeft/internal/layout"
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

type fileReader interface {
	ReadFile(name string) ([]byte, error)
}

type outputWriter interface {
	MkdirAll(path string, perm fs.FileMode) error
	WriteFile(name string, data []byte, perm fs.FileMode) error
}

type pipelineLoader interface {
	Get(name string) (pipeline.Pipe, error)
}

type manifestUpdater interface {
	Set(id string, entry ManifestEntry)
	Get(id string) (ManifestEntry, bool)
}

type artifactCache interface {
	Has(key string) bool
	Store(key string, data []byte) error
	Restore(key string, outputPath string) error
}

type builder struct {
	config *BuilderConfig

	sourceFS fileReader
	schemeFS fileReader
	outputFS outputWriter

	layouts  layoutLoader
	pipes    pipelineLoader
	manifest manifestUpdater
	cache    artifactCache
}

func NewBuilder(
	sourceFS fileReader, schemeFS fileReader, outputFS outputWriter,
	layouts layoutLoader,
	pipes pipelineLoader,
	manifest manifestUpdater,
	cas artifactCache,
	config *BuilderConfig,
) Builder {
	if config == nil {
		config = &BuilderConfig{}
	}
	if config.Concurrency <= 0 {
		config.Concurrency = runtime.NumCPU()
	}
	return &builder{
		config:   config,
		sourceFS: sourceFS,
		schemeFS: schemeFS,
		outputFS: outputFS,

		layouts:  layouts,
		pipes:    pipes,
		manifest: manifest,
		cache:    cas,
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
	stats.Scheduled = len(entries)

	log.IInfo(log.Build).
		Int("total", stats.Scheduled).
		Int64("built", stats.Completed).
		Int64("cache_hits", stats.CacheHits).
		Int("errors", len(stats.Errors)).
		Dur("duration", stats.Duration).
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
	Skipped    bool
}

func (b *builder) runBuild(
	ctx context.Context,
	entries []*BuildEntry,
) BuildStats {

	metrics := &buildMetrics{start: time.Now()}

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

	buildWg.Wait()
	close(writeChan)
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

	sendErr := func(err error) {
		select {
		case <-ctx.Done():
			return
		case errChan <- err:
		}
	}

	for range b.config.Concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				var entry *BuildEntry
				var ok bool
				select {
				case <-ctx.Done():
					return
				case entry, ok = <-in:
					if !ok {
						return
					}
				}
				inCache := b.config.UseCAS && b.cache != nil && !b.config.ForceRebuild && b.cache.Has(entry.Hash)

				if inCache {
					if me, ok := b.manifest.Get(entry.ID); ok && me.Hash == entry.Hash {
						artifact := &buildArtifact{
							Entry:      entry,
							OutputPath: me.OutputPath,
							Cached:     true,
						}

						metrics.cached.Add(1)
						metrics.built.Add(1)

						select {
						case <-ctx.Done():
							return
						case out <- artifact:
						}
						continue
					}
				}

				artifact := b.buildEntry(ctx, entry)

				if artifact.Skipped {
					metrics.skipped.Add(1)
					return
				}

				metrics.built.Add(1)

				if artifact.Error != nil {
					sendErr(artifact.Error)
					continue
				}

				select {
				case out <- artifact:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return &wg
}

func (b *builder) inputFS(root SourceType) (fileReader, error) {
	switch root {
	case SourceTypeSource:
		return b.sourceFS, nil
	case SourceTypeScheme:
		return b.schemeFS, nil
	default:
		return nil, fmt.Errorf("unknown source root: %v", root)
	}
}

// @ts type global.tCtx
// @doc Context passed into the `transform` function.
// @field target: string - Output path for the file being built.
// @field file: FileInfo - Metadata about the source file being transformed.
// @field vars: Record<string, unknown> - Custom key/value mappings loaded from `config.yaml`. Values may include environment-variable references (e.g. `$HOME/docs`), which are expanded at runtime.
// @field type: string - Entry type; one of `"content"` or `"asset"`.
// @field tag: string - Name of the content slot this entry fills in its layout template.
func (b *builder) buildEntry(
	ctx context.Context,
	entry *BuildEntry,
) *buildArtifact {
	start := time.Now()
	artifact := &buildArtifact{Entry: entry}

	inputFS, err := b.inputFS(entry.SourceRoot)
	if err != nil {
		artifact.Error = err
		return artifact
	}

	data, err := inputFS.ReadFile(entry.SourcePath)
	if err != nil {
		artifact.Error = fmt.Errorf("read %s: %w", entry.SourcePath, err)
		return artifact
	}

	pl, err := b.pipes.Get(entry.Pipeline)
	if err != nil {
		if errors.Is(err, pipeline.ErrPipelineNotFound) {
			log.WWarn(log.Build).
				Str("pipe", entry.Pipeline).
				Str("source", entry.SourcePath).
				Err(err).
				Msg("pipeline not found, skipping entry")
			artifact.Skipped = true
			return artifact
		}
		artifact.Error = fmt.Errorf("get pipeline %s: %w", entry.Pipeline, err)
		return artifact
	}

	// TODO define "default" vars keys somewhere, so that ExecuteString can ignore them
	vars := pipeline.Vars{
		"file": pipeline.NewFileInfo(entry.SourcePath, entry.BaseDir),
		"vars": b.config.Variables,
		"type": entry.Type.String(),
		"tag":  entry.TagName,
	}

	for k, v := range entry.ContentVars {
		vars[k] = v
	}

	result, err := pl.Transform(ctx, data, vars)
	if err != nil {
		artifact.Error = fmt.Errorf("transform %s: %w", entry.SourcePath, err)
		return artifact
	}

	lyt, err := b.layouts.Parse(entry.LayoutKey)
	if err != nil {
		artifact.Error = fmt.Errorf("parse layout %s: %w", entry.LayoutKey, err)
		return artifact
	}

	outputPath, err := lyt.ContentOutput(entry.TagName, layout.OutputEntry{
		SourcePath: entry.SourcePath,
		Pipeline:   entry.Pipeline,
		Hash:       entry.Hash,
		BaseDir:    entry.BaseDir,
		Alias:      result.Alias,
	})
	if err != nil {
		artifact.Error = fmt.Errorf("compute output path: %w", err)
		return artifact
	}

	artifact.OutputPath = outputPath

	// For content entries, apply layout template
	if entry.Type == ContentType && lyt != nil {
		vars.Set(entry.TagName, result.Content)

		// render whole layout
		lt, err := b.layouts.Get(ctx, entry.LayoutKey, entry.ContentVars)
		if err != nil {
			artifact.Error = fmt.Errorf("get layout %s: %w", entry.LayoutKey, err)
			return artifact
		}

		artifact.Data = lt.ExecuteBytes(vars)
	} else {
		artifact.Data = result.Content
	}

	elapsed := time.Since(start)
	log.LevelIf(log.Build, elapsed > 2*time.Second, zerolog.WarnLevel, zerolog.TraceLevel).
		Str("source", entry.SourcePath).
		Str("output", outputPath).
		Str("pipe", entry.Pipeline).
		Int("bytes", len(artifact.Data)).
		Dur("duration", elapsed).
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

	if err := b.outputFS.MkdirAll(filepath.Dir(artifact.OutputPath), OutputDirPerm); err != nil {
		return fmt.Errorf("create dir for %s: %w", artifact.OutputPath, err)
	}

	if artifact.Cached {
		if err := b.cache.Restore(entry.Hash, artifact.OutputPath); err != nil {
			return fmt.Errorf("restore from CAS %s: %w", artifact.OutputPath, err)
		}
	} else {
		if b.config.UseCAS {
			if b.cache == nil {
				return errors.New("CAS enabled but not configured")
			}
			if err := b.cache.Store(entry.Hash, artifact.Data); err != nil {
				return fmt.Errorf("store in CAS %s: %w", entry.ID, err)
			}
			if err := b.cache.Restore(entry.Hash, artifact.OutputPath); err != nil {
				return fmt.Errorf("link from CAS %s: %w", artifact.OutputPath, err)
			}
		} else {
			if err := b.outputFS.WriteFile(artifact.OutputPath, artifact.Data, OutputFilePerm); err != nil {
				return fmt.Errorf("write file %s: %w", artifact.OutputPath, err)
			}
		}
	}

	log.TTrace(log.Build).
		Str("entry", entry.ID).
		Str("output", artifact.OutputPath).
		Bool("cached", artifact.Cached).
		Msg("artifact written")

	return nil
}
