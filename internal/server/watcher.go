package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"craeft/internal/log"
	"craeft/internal/project"
	"craeft/internal/utils"

	"github.com/rjeczalik/notify"
)

// FileChange represents a file system change event
type FileChange struct {
	Path string
	Type FileType
	Mod  ModType
	Time time.Time
}

// Watch watches all relevant directories with a unified debounced rebuild
func (s *Server) Watch(ctx context.Context) error {
	changes := make(chan FileChange, 256)
	events := make(chan notify.EventInfo, 256)

	absProjectRoot, err := filepath.Abs(s.config.SourcePath)
	if err != nil {
		return fmt.Errorf("get project root: %w", err)
	}

	absSourcePath, _ := filepath.Abs(s.config.SourcePath)
	absSchemePath, _ := filepath.Abs(s.config.SchemePath)
	absPipesPath, _ := filepath.Abs(s.config.PipesPath)

	if err := notify.Watch(absProjectRoot+"/...", events, notify.All); err != nil {
		return fmt.Errorf("watch project root: %w", err)
	}

	log.IInfo(log.Server).
		Str("root", absProjectRoot).
		Msg("watching all project files")

	defer func() {
		_ = utils.StopComponent(ctx, "fs-watcher", func(context.Context) error {
			notify.Stop(events)
			return nil
		})
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				close(changes)
				return
			case ei := <-events:
				change := s.classifyEvent(ei, absSourcePath, absSchemePath, absPipesPath)
				if change.Type == FileTypeUnknown {
					continue
				}

				merged := s.coalesceFileEvents(change)
				if merged == nil {
					continue
				}

				log.TTrace(log.Server).
					Str("path", change.Path).
					Str("type", string(change.Type)).
					Str("mod", string(change.Mod)).
					Msg("file event classified")

				select {
				case changes <- *merged:
				case <-ctx.Done():
					close(changes)
					return
				}
			}
		}
	}()

	s.processChanges(ctx, changes)
	return nil
}

// eventMergeWindow defines how long two events for the same file
// are considered part of the same save operation.
const eventMergeWindow = 300 * time.Millisecond

// recentEvent tracks recent modifications for a given file
type recentEvent struct {
	LastMod  ModType
	LastTime time.Time
}

// eventCache stores recent events by path
var eventCache sync.Map // path -> *recentEvent

// coalesceFileEvents merges redundant or rapid events for the same file.
// this can happen when editors when changing a file e.g. first save a tmp file, then create a new one, and finally rename them
// Returns nil if the event should be suppressed.
func (s *Server) coalesceFileEvents(change FileChange) *FileChange {
	now := time.Now()

	v, ok := eventCache.Load(change.Path)
	if !ok {
		eventCache.Store(change.Path, &recentEvent{LastMod: change.Mod, LastTime: now})
		return &change
	}

	st := v.(*recentEvent)
	if now.Sub(st.LastTime) < eventMergeWindow {
		st.LastMod = s.resolveEventType(st.LastMod, change.Mod)
		st.LastTime = now
		eventCache.Store(change.Path, st)
		return nil
	}

	eventCache.Store(change.Path, &recentEvent{LastMod: change.Mod, LastTime: now})
	return &change
}

// resolveEventType fuses two modification types into one
func (s *Server) resolveEventType(prev, curr ModType) ModType {
	switch {
	case prev == ModTypeDeleted && curr == ModTypeCreated:
		return ModTypeModified // replaced
	case prev == ModTypeCreated && curr == ModTypeModified:
		return ModTypeCreated // still new file
	case prev == ModTypeCreated && curr == ModTypeDeleted:
		return ModTypeDeleted // short-lived file
	case prev == ModTypeModified && curr == ModTypeModified:
		return ModTypeModified
	case prev == ModTypeDeleted && curr == ModTypeDeleted:
		return ModTypeDeleted
	case curr == ModTypeDeleted:
		return ModTypeDeleted
	default:
		return curr
	}
}

// classifyEvent determines the type of file change based on path and event
func (s *Server) classifyEvent(ei notify.EventInfo, absSource, absScheme, absPipes string) FileChange {
	absPath := ei.Path()
	event := ei.Event()

	var relPath string
	var root string

	if strings.HasPrefix(absPath, absScheme) {
		relPath, _ = filepath.Rel(absScheme, absPath)
		root = "scheme"
	} else if strings.HasPrefix(absPath, absPipes) {
		relPath, _ = filepath.Rel(absPipes, absPath)
		root = "pipes"
	} else if strings.HasPrefix(absPath, absSource) {
		relPath, _ = filepath.Rel(absSource, absPath)
		root = "source"
	} else {
		return FileChange{Type: FileTypeUnknown}
	}

	relPath = filepath.ToSlash(relPath)

	if root == "source" && s.project.IgnoreFile(relPath) {
		return FileChange{Type: FileTypeUnknown}
	}

	ftype := s.classifier.Classify(relPath, event, root)

	return FileChange{
		Path: ftype.Path,
		Type: ftype.Type,
		Mod:  ftype.Mod,
		Time: time.Now(),
	}
}

// processChanges debounces changes and triggers incremental rebuilds
func (s *Server) processChanges(ctx context.Context, changes <-chan FileChange) {
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

		if err := s.handleChangeBatch(ctx, batch); err != nil && !errors.Is(err, context.Canceled) {
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

// handleChangeBatch processes a batch of file changes using the event registry
func (s *Server) handleChangeBatch(ctx context.Context, changes []FileChange) error {
	start := time.Now()

	plan, err := s.project.CurrentPlan()
	if err != nil {
		return fmt.Errorf("get current plan: %w", err)
	}

	grouped := s.groupChanges(changes)

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

	keys := plan.EntryKeys(func(e *project.BuildEntry) bool {
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
		Int64("built", stats.BuiltEntries).
		Int64("cache_hits", stats.CacheHits).
		Int64("errors", int64(len(stats.Errors))).
		Dur("duration", time.Since(start)).
		Msg("rebuild completed")

	if s.config.FsWatcher.HotReload && stats.BuiltEntries > 0 {
		s.reloadPage()
	}

	return nil
}

type changeKey struct {
	fileType FileType
	modType  ModType
}

// groupChanges groups changes by file type and modification type, deduplicating paths
func (s *Server) groupChanges(changes []FileChange) map[changeKey][]string {
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
