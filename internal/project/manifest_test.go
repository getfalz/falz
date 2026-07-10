package project

import (
	"testing"

	"craeft/internal/utils"
)

func newTestRootFS(t *testing.T) *utils.RootFS {
	t.Helper()
	root, err := utils.NewRootFs(t.TempDir())
	if err != nil {
		t.Fatalf("new root fs: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func TestManifest_OpenManifest_MissingFile_StartsEmpty(t *testing.T) {
	root := newTestRootFS(t)

	m, err := OpenManifest(root, "manifest.json")
	if err != nil {
		t.Fatalf("OpenManifest: %v", err)
	}
	if len(m.Entries) != 0 {
		t.Fatalf("expected empty manifest, got %+v", m.Entries)
	}
}

func TestManifest_SetGet(t *testing.T) {
	root := newTestRootFS(t)
	m, err := OpenManifest(root, "manifest.json")
	if err != nil {
		t.Fatalf("OpenManifest: %v", err)
	}

	m.Set("about.md:body", ManifestEntry{Hash: "h1", OutputPath: "about.html"})

	entry, ok := m.Get("about.md:body")
	if !ok || entry.Hash != "h1" || entry.OutputPath != "about.html" {
		t.Fatalf("unexpected entry: %+v (ok=%v)", entry, ok)
	}

	if _, ok := m.Get("missing"); ok {
		t.Fatalf("expected missing entry to not be found")
	}
}

func TestManifest_Delete(t *testing.T) {
	root := newTestRootFS(t)
	m, err := OpenManifest(root, "manifest.json")
	if err != nil {
		t.Fatalf("OpenManifest: %v", err)
	}

	m.Set("about.md:body", ManifestEntry{Hash: "h1"})
	m.Delete("about.md:body")

	if _, ok := m.Get("about.md:body"); ok {
		t.Fatalf("expected entry to be deleted")
	}
}

func TestManifest_IsDirty(t *testing.T) {
	root := newTestRootFS(t)
	m, err := OpenManifest(root, "manifest.json")
	if err != nil {
		t.Fatalf("OpenManifest: %v", err)
	}

	if !m.IsDirty("unknown", "h1") {
		t.Fatalf("expected an absent entry to be dirty")
	}

	m.Set("about.md:body", ManifestEntry{Hash: "h1"})

	if m.IsDirty("about.md:body", "h1") {
		t.Fatalf("expected matching hash to not be dirty")
	}
	if !m.IsDirty("about.md:body", "h2") {
		t.Fatalf("expected mismatched hash to be dirty")
	}
}

func TestManifest_CleanStale_RemovesAndReturnsOnlyMissingIDs(t *testing.T) {
	root := newTestRootFS(t)
	m, err := OpenManifest(root, "manifest.json")
	if err != nil {
		t.Fatalf("OpenManifest: %v", err)
	}

	m.Set("keep.md:body", ManifestEntry{Hash: "h1", OutputPath: "keep.html"})
	m.Set("stale.md:body", ManifestEntry{Hash: "h2", OutputPath: "stale.html"})

	removed := m.CleanStale(map[string]struct{}{"keep.md:body": {}})

	if len(removed) != 1 || removed[0].OutputPath != "stale.html" {
		t.Fatalf("expected only the stale entry to be removed, got %+v", removed)
	}
	if _, ok := m.Get("stale.md:body"); ok {
		t.Fatalf("expected stale entry to be gone from the manifest")
	}
	if _, ok := m.Get("keep.md:body"); !ok {
		t.Fatalf("expected kept entry to still be present")
	}
}

func TestManifest_Save_ThenOpenManifest_RoundTrips(t *testing.T) {
	root := newTestRootFS(t)
	m, err := OpenManifest(root, "manifest.json")
	if err != nil {
		t.Fatalf("OpenManifest: %v", err)
	}

	m.Set("about.md:body", ManifestEntry{Hash: "h1", OutputPath: "about.html"})

	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reopened, err := OpenManifest(root, "manifest.json")
	if err != nil {
		t.Fatalf("OpenManifest (reopen): %v", err)
	}

	entry, ok := reopened.Get("about.md:body")
	if !ok || entry.Hash != "h1" || entry.OutputPath != "about.html" {
		t.Fatalf("expected persisted entry to round-trip, got %+v (ok=%v)", entry, ok)
	}
}
