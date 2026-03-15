package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifest_NewManifest(t *testing.T) {
	m := NewManifest()

	if m.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", m.Version)
	}
	if m.Entries == nil {
		t.Error("Expected non-nil entries map")
	}
	if m.Updated.IsZero() {
		t.Error("Expected non-zero updated time")
	}
}

func TestManifest_SetAndGet(t *testing.T) {
	m := NewManifest()

	entry := ManifestEntry{
		Hash:       "abc123",
		OutputPath: "test.html",
	}

	m.Set("test.md", entry)

	retrieved, ok := m.Get("test.md")
	if !ok {
		t.Fatal("Expected to find entry")
	}

	if retrieved.Hash != entry.Hash {
		t.Errorf("Expected hash %s, got %s", entry.Hash, retrieved.Hash)
	}
	if retrieved.OutputPath != entry.OutputPath {
		t.Errorf("Expected path %s, got %s", entry.OutputPath, retrieved.OutputPath)
	}
}

func TestManifest_Delete(t *testing.T) {
	m := NewManifest()

	m.Set("test.md", ManifestEntry{Hash: "abc"})
	m.Delete("test.md")

	_, ok := m.Get("test.md")
	if ok {
		t.Error("Expected entry to be deleted")
	}
}

func TestManifest_IsDirty(t *testing.T) {
	m := NewManifest()

	// Non-existent entry should be dirty
	if !m.IsDirty("new.md", "hash123") {
		t.Error("Expected non-existent entry to be dirty")
	}

	// Add entry
	m.Set("test.md", ManifestEntry{Hash: "hash123", OutputPath: "test.html"})

	// Same hash should not be dirty
	if m.IsDirty("test.md", "hash123") {
		t.Error("Expected entry with same hash to not be dirty")
	}

	// Different hash should be dirty
	if !m.IsDirty("test.md", "hash456") {
		t.Error("Expected entry with different hash to be dirty")
	}

	if !m.IsDirty("test.md", "hash456") {
		t.Error("Expected entry with different output path to be dirty")
	}
}

func TestManifest_CleanStale(t *testing.T) {
	m := NewManifest()

	m.Set("keep.md", ManifestEntry{Hash: "a"})
	m.Set("remove1.md", ManifestEntry{Hash: "b"})
	m.Set("remove2.md", ManifestEntry{Hash: "c"})

	currentIDs := map[string]struct{}{
		"keep.md": {},
	}

	removed := m.CleanStale(currentIDs)

	if len(removed) != 2 {
		t.Errorf("Expected 2 removed entries, got %d", len(removed))
	}

	// Verify keep.md still exists
	if _, ok := m.Get("keep.md"); !ok {
		t.Error("Expected keep.md to still exist")
	}

	// Verify others are removed
	if _, ok := m.Get("remove1.md"); ok {
		t.Error("Expected remove1.md to be removed")
	}
	if _, ok := m.Get("remove2.md"); ok {
		t.Error("Expected remove2.md to be removed")
	}
}

func TestManifest_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "manifest.json")

	// Create and save manifest
	m1 := NewManifest()
	m1.Set("test.md", ManifestEntry{
		Hash:       "abc123",
		OutputPath: "test.html",
	})

	err := m1.Save(manifestPath)
	if err != nil {
		t.Fatalf("Failed to save manifest: %v", err)
	}

	// Load manifest
	m2, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("Failed to load manifest: %v", err)
	}

	// Verify data
	entry, ok := m2.Get("test.md")
	if !ok {
		t.Fatal("Expected to find entry in loaded manifest")
	}

	if entry.Hash != "abc123" {
		t.Errorf("Expected hash abc123, got %s", entry.Hash)
	}
}

func TestManifest_LoadNonExistent(t *testing.T) {
	m, err := LoadManifest("/nonexistent/manifest.json")
	if err != nil {
		t.Fatalf("Expected no error for non-existent manifest: %v", err)
	}

	if m == nil {
		t.Fatal("Expected new manifest to be created")
	}

	if len(m.Entries) != 0 {
		t.Error("Expected empty entries in new manifest")
	}
}

func TestManifest_SaveAtomically(t *testing.T) {
	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "manifest.json")

	m := NewManifest()
	m.Set("test.md", ManifestEntry{Hash: "abc"})

	err := m.Save(manifestPath)
	if err != nil {
		t.Fatalf("Failed to save: %v", err)
	}

	// Verify no .tmp file left behind
	tmpPath := manifestPath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("Expected temporary file to be removed")
	}
}
