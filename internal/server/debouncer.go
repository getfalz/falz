package server

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/project"
	"errors"
	"fmt"
	"sync"
	"time"
)

type changeKey struct {
	fileType FileType
	modType  ModType
}

type DebouncerConfig struct {
	HotReload bool
}

type Debouncer struct {
	config     DebouncerConfig
	registry   *EventRegistry
	classifier *FileClassifier
	hr         reloader
	project    *project.Project
}

type reloader interface {
	ReloadPage()
	NavigateTo(url string)
}

func NewDebouncer(config DebouncerConfig, p *project.Project, registry *EventRegistry, classifier *FileClassifier, hr reloader) *Debouncer {
	return &Debouncer{
		config:     config,
		registry:   registry,
		hr:         hr,
		classifier: classifier,
		project:    p,
	}
}

func (d *Debouncer) Run(ctx context.Context, changes <-chan RawChange) {
	const debounceWindow = 150 * time.Millisecond

	workCh := make(chan []RawChange, 1)

	// Worker
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case batch := <-workCh:
				if err := d.handleBatch(ctx, batch); err != nil && !errors.Is(err, context.Canceled) {
					log.EError(log.Server).
						Err(err).
						Int("batch_size", len(batch)).
						Msg("rebuild failed")
				}
			}
		}
	}()

	var (
		mu      sync.Mutex
		pending []RawChange
		timer   *time.Timer
	)

	flush := func() {
		mu.Lock()
		defer mu.Unlock()

		if len(pending) == 0 {
			return
		}

		batch := pending
		pending = nil

		select {
		case workCh <- batch:
		default:
			waiting := <-workCh
			workCh <- append(waiting, batch...)
			log.DDebug(log.Server).
				Int("new_batch", len(batch)).
				Int("already_waiting", len(waiting)).
				Msg("worker busy, changes queued into waiting batch")
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case raw, ok := <-changes:
			if !ok {
				return
			}
			mu.Lock()
			pending = append(pending, raw)
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounceWindow, flush)
			mu.Unlock()
		}
	}
}

// handleBatch processes a batch of file changes using the event registry
func (d *Debouncer) handleBatch(ctx context.Context, raws []RawChange) error {
	start := time.Now()

	classified := make([]FileChange, 0, len(raws))
	for _, raw := range raws {
		fc := d.classifier.Classify(ctx, raw)
		if fc == nil {
			continue
		}
		classified = append(classified, *fc)
	}

	if len(classified) == 0 {
		log.DDebug(log.Server).
			Int("raw_changes", len(raws)).
			Msg("batch fully filtered after classification, skipping rebuild")
		return nil
	}

	grouped := groupByHandler(classified)

	log.DDebug(log.Server).
		Int("raw", len(raws)).
		Int("classified", len(classified)).
		Int("filtered", len(raws)-len(classified)).
		Int("handlers", len(grouped)).
		Msg("dispatching change batch")

	for key, paths := range grouped {
		log.TTrace(log.Server).
			Str("type", string(key.fileType)).
			Str("mod", string(key.modType)).
			Strs("paths", paths).
			Msg("dispatching to handler")

		if err := d.registry.Dispatch(ctx, key.fileType, key.modType, paths); err != nil {
			log.EError(log.Server).
				Err(err).
				Str("type", string(key.fileType)).
				Str("mod", string(key.modType)).
				Msg("handler failed")
			// continue: one handler failure must not abort the others
		}
	}

	dirty := d.project.Graph().DirtyEntries()
	if len(dirty) == 0 {
		log.DDebug(log.Server).
			Int("classified", len(classified)).
			Strs("paths", pathsFromChanges(classified)).
			Msg("no dirty entries after dispatch, skipping rebuild")
		return nil
	}

	log.IInfo(log.Server).
		Int("dirty", len(dirty)).
		Msg("starting incremental rebuild")

	stats, err := d.project.IncrementalBuild(ctx)
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("incremental build: %w", err)
	}

	log.IInfo(log.Server).
		Int64("built", stats.Completed).
		Int64("cache", stats.CacheHits).
		Int64("errors", int64(len(stats.Errors))).
		Dur("duration", time.Since(start)).
		Msg("rebuild completed")

	if d.config.HotReload && stats.Completed > 0 && d.hr != nil {
		d.notify(classified, dirty)
	}

	return nil
}

func groupByHandler(changes []FileChange) map[changeKey][]string {
	grouped := make(map[changeKey][]string, len(changes))
	for _, change := range changes {
		key := changeKey{change.Type, change.Mod}
		grouped[key] = append(grouped[key], change.Path)
	}
	return grouped
}

func pathsFromChanges(changes []FileChange) []string {
	paths := make([]string, len(changes))
	for i, c := range changes {
		paths[i] = c.Path
	}
	return paths
}

func (d *Debouncer) notify(changes []FileChange, dirty []*project.BuildEntry) {
	for _, c := range changes {
		switch c.Type {
		case FileTypeLayout, FileTypeFragment, FileTypePipeline, FileTypeSchemeAsset:
			log.DDebug(log.Server).
				Str("reason", string(c.Type)).
				Msg("hot-reload: full page reload (structural change)")
			d.hr.ReloadPage()
			return
		}
	}

	var contentEntries []*project.BuildEntry
	for _, e := range dirty {
		if e.Type == project.ContentType {
			contentEntries = append(contentEntries, e)
		}
	}

	if len(contentEntries) == 1 {
		entry := contentEntries[0]
		me, ok := d.project.Manifest.Get(entry.ID)
		if ok && me.OutputPath != "" {
			url := "/" + me.OutputPath
			log.DDebug(log.Server).
				Str("entry", entry.ID).
				Str("url", url).
				Msg("hot-reload: navigate to rebuilt page")
			d.hr.NavigateTo(url)
			return
		}
	}

	log.DDebug(log.Server).
		Int("content_entries", len(contentEntries)).
		Msg("hot-reload: full page reload")
	d.hr.ReloadPage()
}
