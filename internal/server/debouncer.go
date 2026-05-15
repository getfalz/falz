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
	config   DebouncerConfig
	registry *EventRegistry
	hr       reloader
	project  *project.Project
}

type reloader interface {
    ReloadPage()
    NavigateTo(url string)
}

func NewDebouncer(config DebouncerConfig, p *project.Project, registry *EventRegistry, hr reloader) *Debouncer {
	return &Debouncer{
		config:   config,
		registry: registry,
		hr:       hr,
		project:  p,
	}
}

func (d *Debouncer) Run(ctx context.Context, changes <-chan FileChange) {
	const debounceWindow = 150 * time.Millisecond

	var (
		mu             sync.Mutex
		pendingChanges []FileChange
		timer          *time.Timer
		rebuilding     bool
	)

	var triggerRebuild func()
	triggerRebuild = func() {
		mu.Lock()
		if rebuilding {
			timer = time.AfterFunc(debounceWindow, triggerRebuild)
			mu.Unlock()
			return
		}

		rebuilding = true
		batch := pendingChanges
		pendingChanges = nil
		mu.Unlock()

		if len(batch) > 0 {
			log.DDebug(log.Server).
				Int("changes", len(batch)).
				Msg("processing change batch")

			if err := d.handleBatch(ctx, batch); err != nil && !errors.Is(err, context.Canceled) {
				log.EError(log.Server).Err(err).Msg("rebuild failed")
			}
		}

		mu.Lock()
		rebuilding = false
		mu.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case change, ok := <-changes:
			if !ok {
				return
			}

			mu.Lock()
			pendingChanges = append(pendingChanges, change)

			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounceWindow, triggerRebuild)
			mu.Unlock()
		}
	}
}

// handleBatch processes a batch of file changes using the event registry
func (d *Debouncer) handleBatch(ctx context.Context, changes []FileChange) error {
	start := time.Now()

	grouped := groupChanges(changes)

	log.IInfo(log.Server).
		Int("total_changes", len(changes)).
		Int("unique_handlers", len(grouped)).
		Msg("processing file changes")

	for key, paths := range grouped {
		if len(paths) == 0 {
			continue
		}

		log.TTrace(log.Server).
			Str("type", string(key.fileType)).
			Str("mod", string(key.modType)).
			Int("count", len(paths)).
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
		log.TTrace(log.Server).
			Int("changes", len(changes)).
			Msg("no dirty entries after dispatch, skipping rebuild")
		return nil
	}

	stats, err := d.project.IncrementalBuild(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("incremental build: %w", err)
	}

	log.IInfo(log.Server).
		Int64("built", stats.Completed).
		Int64("cached", stats.CacheHits).
		Int64("errors", int64(len(stats.Errors))).
		Dur("duration", time.Since(start)).
		Msg("incremental rebuild")

	if d.config.HotReload && stats.Completed > 0 && d.hr != nil {
		d.notify(changes, dirty)
	}

	return nil
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

// groupChanges groups changes by file type and modification type, deduplicating paths
func groupChanges(changes []FileChange) map[changeKey][]string {
	grouped := make(map[changeKey][]string)
	seen := make(map[changeKey]map[string]bool)

	for _, change := range changes {
		key := changeKey{
			fileType: change.Type,
			modType:  change.Mod,
		}

		if seen[key] == nil {
			seen[key] = make(map[string]bool)
		}

		if !seen[key][change.Path] {
			seen[key][change.Path] = true
			grouped[key] = append(grouped[key], change.Path)
		}
	}

	return grouped
}
