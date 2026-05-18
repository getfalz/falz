package server

import (
	"craeft/internal/log"
	"sync"
	"time"
)

type EventCoalescer struct {
	mu    sync.Mutex
	cache map[string]*recentEvent
}

func NewEventCoalescer() *EventCoalescer {
	return &EventCoalescer{
		cache: make(map[string]*recentEvent),
	}
}

// eventMergeWindow defines how long two events for the same file
// are considered part of the same save operation.
const eventMergeWindow = 500 * time.Millisecond

// recentEvent tracks recent modifications for a given file
type recentEvent struct {
	mod      ModType
	lastTime time.Time
}

// Coalesce resolves the logical modification type for an incoming event
// by combining it with any recent event for the same path within the merge
// window.
func (c *EventCoalescer) Coalesce(change *RawChange) *RawChange {
	now := time.Now()

	c.mu.Lock()
	prev, ok := c.cache[change.Path]
	if ok && now.Sub(prev.lastTime) >= eventMergeWindow {
		// Previous window expired; treat this as a fresh event.
		ok = false
	}

	resolvedMod := change.Mod
	if ok {
		resolvedMod = resolveModType(prev.mod, change.Mod)
	}

	c.cache[change.Path] = &recentEvent{mod: resolvedMod, lastTime: now}
	c.mu.Unlock()

	// supress the event if it resolves to the same modification type as the previous one
	if ok && resolvedMod == prev.mod {
		log.TTrace(log.Server).
			Str("path", change.Path).
			Str("mod", string(change.Mod)).
			Str("prev_mod", string(prev.mod)).
			Str("cached_mod", string(resolvedMod)).
			Bool("forwarded", false).
			Msg("file event")
		return nil
	}

	// forward the event with the resolved modification type if it differs from the previous one
	if resolvedMod != change.Mod {
		log.TTrace(log.Server).
			Str("path", change.Path).
			Str("event", string(change.Mod)).
			Str("last_event", string(prev.mod)).
			Str("resolved_event", string(resolvedMod)).
			Bool("forwarded", true).
			Msg("file event")
		out := *change
		out.Mod = resolvedMod
		return &out
	}

	// no prior event or same modification type, forward as is
	log.TTrace(log.Server).
		Str("path", change.Path).
		Str("event", string(change.Mod)).
		Str("resolved_event", string(resolvedMod)).
		Bool("forwarded", true).
		Msg("file event")

	return change
}

// resolveModType fuses two consecutive modification types into one logical op.
//
// Real-world editor save sequences on Linux (inotify → notify library):
//
//	Vim/Neovim default:         write + MOVED_TO(=create)  → modified
//	VSCode/JetBrains/Sublime:   MOVED_TO(=create) only     → handler fallback
//	Gedit:                      write + remove + create     → modified
//	Nano/Emacs:                 write + write               → modified
//	Vim backupcopy=yes:         write only                  → modified
func resolveModType(prev, curr ModType) ModType {
	switch {
	case prev == ModTypeCreated && curr == ModTypeModified:
		return ModTypeCreated // new file written to — still a new file
	case prev == ModTypeCreated && curr == ModTypeDeleted:
		return ModTypeDeleted // transient file, never existed from our perspective
	case prev == ModTypeDeleted && curr == ModTypeCreated:
		return ModTypeModified // deleted then recreated — net effect is a modify
	case prev == ModTypeModified && curr == ModTypeCreated:
		return ModTypeModified // atomic rename write (write + MOVED_TO) — still a modify
	case curr == ModTypeDeleted:
		return ModTypeDeleted // deletion always wins
	default:
		return curr
	}
}
