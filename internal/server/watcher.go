package server

import (
	"context"
	"fmt"
	"path/filepath"
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

type WatcherConfig struct {
	SourcePath string
	Enabled    bool // Content || Theme || Pipes
}

type FSWatcher struct {
	config     WatcherConfig
	classifier *FileClassifier
	coalescer  *EventCoalescer
}

func NewFsWatcher(config WatcherConfig, classifier *FileClassifier, coalescer *EventCoalescer) *FSWatcher {
	return &FSWatcher{
		config:     config,
		classifier: classifier,
		coalescer:  coalescer,
	}
}

// Watch watches all relevant directories with a unified debounced rebuild
func (f *FSWatcher) Run(ctx context.Context, changes chan FileChange) error {
	events := make(chan notify.EventInfo, 256)

	absProjectRoot, err := filepath.Abs(f.config.SourcePath)
	if err != nil {
		return fmt.Errorf("get project root: %w", err)
	}

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

	for {
		select {
		case <-ctx.Done():
			return nil
		case ei, ok := <-events:
			if !ok {
				return nil
			}

			// log.TTrace(log.Server).
			// 	Str("path", ei.Path()).
			// 	Str("raw-event", ei.Event().String()).
			// 	Msg("raw fs event")

			change := f.classifier.Classify(ei)
			if change == nil {
				continue
			}

			merged := f.coalescer.Coalesce(change)
			if merged == nil {
				continue
			}

			log.TTrace(log.Server).
				Str("path", merged.Path).
				Str("type", string(merged.Type)).
				Str("event", string(merged.Mod)).
				Msg("file event classified")

			select {
			case changes <- *merged:
			case <-ctx.Done():
				return nil
			}
		}
	}
}
