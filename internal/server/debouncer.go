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

type Debouncer struct {
}

type changeKey struct {
	fileType FileType
	modType  ModType
}

// processChanges debounces changes and triggers incremental rebuilds
func (d *Debouncer) Run(ctx context.Context, changes <-chan FileChange) {
	const debounceWindow = 150 * time.Millisecond

	var (
		mu             sync.Mutex
		pendingChanges []FileChange
		timer          *time.Timer
		rebuilding     bool
	)

	triggerRebuild := func() {
		mu.Lock()
		if rebuilding {
			mu.Unlock()
			return
		}
		rebuilding = true
		batch := pendingChanges
		pendingChanges = nil
		mu.Unlock()

		if len(batch) == 0 {
			mu.Lock()
			rebuilding = false
			mu.Unlock()
			return
		}

		log.DDebug(log.Server).
			Int("changes", len(batch)).
			Msg("processing change batch")

		if err := d.handleBatch(ctx, batch); err != nil && !errors.Is(err, context.Canceled) {
			log.EError(log.Server).Err(err).Msg("rebuild failed")
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

	plan, err := s.project.CurrentPlan()
	if err != nil {
		return fmt.Errorf("get current plan: %w", err)
	}

	grouped := d.groupChanges(changes)

	log.IInfo(log.Server).
		Int("total_changes", len(changes)).
		Int("unique_handlers", len(grouped)).
		Msg("processing file changes")

	for key, paths := range grouped {
		if len(paths) == 0 {
			continue
		}

		// log.TTrace(log.Server).
		// 	Str("type", string(key.fileType)).
		// 	Str("mod", string(key.modType)).
		// 	Int("count", len(paths)).
		// 	Msg("dispatching to handler")

		if err := s.eventRegistry.Dispatch(ctx, plan, key.fileType, key.modType, paths); err != nil {
			log.EError(log.Server).
				Err(err).
				Str("type", string(key.fileType)).
				Str("mod", string(key.modType)).
				Msg("handler failed")
		}
	}

	keys := plan.EntryKeys(func(e *project.Entry) bool {
		return e.Dirty
	})
	dirtyCount := len(keys)

	if dirtyCount == 0 {
		log.DDebug(log.Server).
			Int("changes", len(changes)).
			Msg("no dirty entries, skipping rebuild")
		if s.config.FsWatcher.HotReload {
			s.reloadPage()
		}
		return nil
	}

	log.IInfo(log.Server).
		Int("dirty", dirtyCount).
		Msg("starting incremental rebuild")

	stats, err := s.project.BuildFromPlan(ctx, plan)
	if err != nil && !errors.Is(err, context.Canceled) {
		var be *project.BuildError
		if errors.As(err, &be) {
			log.EError(log.Server).
				Err(err).
				Str("stats", stats.String()).
				Msg("incremental build completed with build errors")
		} else {
			return fmt.Errorf("incremental build: %w", err)
		}
	}

	log.IInfo(log.Server).
		Int64("built", stats.Built).
		Int64("cache_hits", stats.CacheHits).
		Int64("errors", int64(len(stats.Errors))).
		Dur("duration", time.Since(start)).
		Msg("rebuild completed")

	if s.config.FsWatcher.HotReload && stats.Built > 0 {
		s.reloadPage()
	}

	return nil
}

// groupChanges groups changes by file type and modification type, deduplicating paths
func (d *Debouncer) groupChanges(changes []FileChange) map[changeKey][]string {
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
