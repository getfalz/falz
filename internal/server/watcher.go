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

type RawChange struct {
	Path string
	Mod  ModType
	Time time.Time
}

// FileChange is the fully classified event that handlers receive.
// It is produced by the debouncer worker after coalescing and batching.
type FileChange struct {
	Path string
	Type FileType
	Mod  ModType
	Time time.Time
}

type FSWatcher struct {
	sourcePath string
	coalescer *EventCoalescer
}

func NewFsWatcher(sourcePath string, coalescer *EventCoalescer) *FSWatcher {
	return &FSWatcher{
		sourcePath: sourcePath,
		coalescer: coalescer,
	}
}

func (f *FSWatcher) Run(ctx context.Context, changes chan RawChange) error {
	events := make(chan notify.EventInfo, 256)

	absProjectRoot, err := filepath.Abs(f.sourcePath)
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

			raw := &RawChange{
				Path: ei.Path(),
				Mod:  resolveMod(ei.Event()),
				Time: time.Now(),
			}

			merged := f.coalescer.Coalesce(raw)
			if merged == nil {
				continue
			}

			select {
			case changes <- *merged:
			case <-ctx.Done():
				return nil
			}
		}
	}
}
