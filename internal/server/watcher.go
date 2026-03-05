package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"craeft/internal/log"
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

type FSWatcher struct {

}

// Watch watches all relevant directories with a unified debounced rebuild
func (f *FSWatcher) Run(ctx context.Context) error {
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

// classifyEvent determines the type of file change based on path and event
func (f *FSWatcher) classifyEvent(ei notify.EventInfo, absSource, absScheme, absPipes string) FileChange {
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
