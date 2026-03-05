package server

import (
	"sync"
	"time"
)

type EventCoalescer struct {

	// eventCache stores recent events by path
	eventCache sync.Map // path -> *recentEvent
}

// eventMergeWindow defines how long two events for the same file
// are considered part of the same save operation.
const eventMergeWindow = 300 * time.Millisecond

// recentEvent tracks recent modifications for a given file
type recentEvent struct {
	LastMod  ModType
	LastTime time.Time
}

// coalesceFileEvents merges redundant or rapid events for the same file.
// this can happen when editors when changing a file e.g. first save a tmp file, then create a new one, and finally rename them
// Returns nil if the event should be suppressed.
func (c *EventCoalescer) Coalesce(change FileChange) *FileChange {
	now := time.Now()

	v, ok := c.eventCache.Load(change.Path)
	if !ok {
		c.eventCache.Store(change.Path, &recentEvent{LastMod: change.Mod, LastTime: now})
		return &change
	}

	st := v.(*recentEvent)
	if now.Sub(st.LastTime) < eventMergeWindow {
		st.LastMod = c.resolveEventType(st.LastMod, change.Mod)
		st.LastTime = now
		c.eventCache.Store(change.Path, st)
		return nil
	}

	c.eventCache.Store(change.Path, &recentEvent{LastMod: change.Mod, LastTime: now})
	return &change
}

// resolveEventType fuses two modification types into one
func (c *EventCoalescer) resolveEventType(prev, curr ModType) ModType {
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
