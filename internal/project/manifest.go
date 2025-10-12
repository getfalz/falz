package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manifest tracks the state of built files
type Manifest struct {
	Version   string                   `json:"version"`
	Entries   map[string]ManifestEntry `json:"entries"`
	entriesMu sync.RWMutex
	Updated   time.Time `json:"updated"`
}

// ManifestEntry represents a single file in the manifest
type ManifestEntry struct {
	Hash       string    `json:"hash"`
	OutputPath string    `json:"output"`
	Built      time.Time `json:"built"`
	Size       int64     `json:"size,omitempty"`
	Type       string    `json:"type,omitempty"` // "content" or "asset"
}

// NewManifest creates a new empty manifest
func NewManifest() *Manifest {
	return &Manifest{
		Version: "1.0",
		Entries: make(map[string]ManifestEntry),
		Updated: time.Now(),
	}
}

// LoadManifest loads a manifest from disk
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewManifest(), nil
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if m.Entries == nil {
		m.Entries = make(map[string]ManifestEntry)
	}

	return &m, nil
}

// Save saves the manifest to disk atomically
func (m *Manifest) Save(path string) error {
	m.entriesMu.Lock()
	m.Updated = time.Now()
	data, err := json.MarshalIndent(m, "", "  ")
	m.entriesMu.Unlock()

	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("write temp manifest: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename manifest: %w", err)
	}

	return nil
}

// Get returns the manifest entry for a given ID
func (m *Manifest) Get(id string) (ManifestEntry, bool) {
	m.entriesMu.RLock()
	defer m.entriesMu.RUnlock()
	entry, ok := m.Entries[id]
	return entry, ok
}

// Set updates or creates a manifest entry
func (m *Manifest) Set(id string, entry ManifestEntry) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()
	m.Entries[id] = entry
}

// Delete removes an entry from the manifest
func (m *Manifest) Delete(id string) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()
	delete(m.Entries, id)
}

// IsDirty checks if an entry needs rebuilding
func (m *Manifest) IsDirty(id string, currentHash string) bool {
	entry, exists := m.Get(id)
	return !exists || entry.Hash != currentHash
}

// CleanStale removes entries that are no longer in the current plan
func (m *Manifest) CleanStale(currentIDs map[string]struct{}) []ManifestEntry {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()

	var removed []ManifestEntry
	for id, entry := range m.Entries {
		if _, exists := currentIDs[id]; !exists {
			removed = append(removed, entry)
			delete(m.Entries, id)
		}
	}
	return removed
}
