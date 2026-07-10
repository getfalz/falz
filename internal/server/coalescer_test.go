package server

import (
	"testing"
	"time"
)

func TestResolveModType(t *testing.T) {
	tests := []struct {
		name string
		prev ModType
		curr ModType
		want ModType
	}{
		{"vim/neovim: write then atomic rename", ModTypeModified, ModTypeCreated, ModTypeModified},
		{"new file written to again", ModTypeCreated, ModTypeModified, ModTypeCreated},
		{"transient file created then deleted", ModTypeCreated, ModTypeDeleted, ModTypeDeleted},
		{"deleted then recreated nets a modify", ModTypeDeleted, ModTypeCreated, ModTypeModified},
		{"deletion always wins", ModTypeModified, ModTypeDeleted, ModTypeDeleted},
		{"two plain modifies stay modified", ModTypeModified, ModTypeModified, ModTypeModified},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveModType(tt.prev, tt.curr); got != tt.want {
				t.Fatalf("resolveModType(%s, %s) = %s, want %s", tt.prev, tt.curr, got, tt.want)
			}
		})
	}
}

func TestCoalesce_FirstEventForPath_IsForwardedAsIs(t *testing.T) {
	c := NewEventCoalescer()

	got := c.Coalesce(&RawChange{Path: "about.md", Mod: ModTypeModified, Time: time.Now()})
	if got == nil || got.Mod != ModTypeModified {
		t.Fatalf("expected the first event for a path to be forwarded as-is, got %+v", got)
	}
}

// TestCoalesce_VimSaveSequence_SuppressesRedundantSecondEvent models a real
// Vim/Neovim save: a write (Modified) immediately followed by an atomic
// rename-in (Created) for the same path. The net perceived state is still
// "modified", so the second raw event should be suppressed rather than
// forwarded a second time.
func TestCoalesce_VimSaveSequence_SuppressesRedundantSecondEvent(t *testing.T) {
	c := NewEventCoalescer()

	first := c.Coalesce(&RawChange{Path: "about.md", Mod: ModTypeModified, Time: time.Now()})
	if first == nil || first.Mod != ModTypeModified {
		t.Fatalf("expected the write event to be forwarded as Modified, got %+v", first)
	}

	second := c.Coalesce(&RawChange{Path: "about.md", Mod: ModTypeCreated, Time: time.Now()})
	if second != nil {
		t.Fatalf("expected the follow-up rename event to be suppressed, got %+v", second)
	}
}

// TestCoalesce_DeleteThenRecreate_ForwardsAsModified models saving via a
// temp-file swap (delete original, create new): the net effect for the
// watcher's consumer is a single modification, not a delete-then-create
// flicker.
func TestCoalesce_DeleteThenRecreate_ForwardsAsModified(t *testing.T) {
	c := NewEventCoalescer()

	if got := c.Coalesce(&RawChange{Path: "about.md", Mod: ModTypeDeleted, Time: time.Now()}); got == nil || got.Mod != ModTypeDeleted {
		t.Fatalf("expected the delete to be forwarded as-is, got %+v", got)
	}

	got := c.Coalesce(&RawChange{Path: "about.md", Mod: ModTypeCreated, Time: time.Now()})
	if got == nil {
		t.Fatalf("expected the recreate to be forwarded")
	}
	if got.Mod != ModTypeModified {
		t.Fatalf("expected delete-then-recreate to resolve to Modified, got %s", got.Mod)
	}
}

// TestCoalesce_EventOutsideMergeWindow_TreatedAsFresh ensures that once the
// merge window has elapsed, a new event for the same path is no longer
// fused with the stale one (e.g. two genuinely separate edits a second
// apart shouldn't be coalesced together).
func TestCoalesce_EventOutsideMergeWindow_TreatedAsFresh(t *testing.T) {
	c := NewEventCoalescer()

	old := time.Now().Add(-2 * eventMergeWindow)
	c.cache["about.md"] = &recentEvent{mod: ModTypeCreated, lastTime: old}

	got := c.Coalesce(&RawChange{Path: "about.md", Mod: ModTypeCreated, Time: time.Now()})
	if got == nil || got.Mod != ModTypeCreated {
		t.Fatalf("expected a stale cache entry to be ignored and the event forwarded as-is, got %+v", got)
	}
}

func TestCoalesce_DifferentPaths_TrackedIndependently(t *testing.T) {
	c := NewEventCoalescer()

	a := c.Coalesce(&RawChange{Path: "a.md", Mod: ModTypeModified, Time: time.Now()})
	b := c.Coalesce(&RawChange{Path: "b.md", Mod: ModTypeModified, Time: time.Now()})

	if a == nil || b == nil {
		t.Fatalf("expected both first-time events on distinct paths to be forwarded, got a=%+v b=%+v", a, b)
	}
}
