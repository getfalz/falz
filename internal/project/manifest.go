package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"craeft/internal/log"
	"craeft/internal/utils"
)

// Manifest tracks the state of built files.
type Manifest struct {
	Version   string                   `json:"version"`
	Entries   map[string]ManifestEntry `json:"entries"`
	Updated   time.Time                `json:"updated"`
	entriesMu sync.RWMutex

	root *utils.RootFS
	name string // relative path within root, e.g. manifestoFile
}

// ManifestEntry represents a single output file tracked by the manifest.
type ManifestEntry struct {
	Hash       string `json:"hash"`
	OutputPath string `json:"output"`
}

func OpenManifest(root *utils.RootFS, name string) (*Manifest, error) {
	data, err := root.ReadFile(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log.TTrace(log.Build).Msg("manifest not found, starting fresh")
			return newManifest(root, name), nil
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
	m.root = root
	m.name = name

	log.TTrace(log.Build).Int("entries", len(m.Entries)).Msg("manifest loaded")
	return &m, nil
}

func newManifest(root *utils.RootFS, name string) *Manifest {
	return &Manifest{
		Version: "1.0",
		Entries: make(map[string]ManifestEntry),
		Updated: time.Now(),
		root:    root,
		name:    name,
	}
}

func (m *Manifest) Save() error {
	m.entriesMu.Lock()
	m.Updated = time.Now()
	data, err := json.MarshalIndent(m, "", "  ")
	m.entriesMu.Unlock()

	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := m.root.WriteFile(m.name, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

// Get returns the manifest entry for the given ID.
func (m *Manifest) Get(id string) (ManifestEntry, bool) {
	m.entriesMu.RLock()
	defer m.entriesMu.RUnlock()
	entry, ok := m.Entries[id]
	return entry, ok
}

// Set creates or updates the manifest entry for the given ID.
func (m *Manifest) Set(id string, entry ManifestEntry) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()
	m.Entries[id] = entry
}

// Delete removes an entry from the manifest.
func (m *Manifest) Delete(id string) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()
	delete(m.Entries, id)
}

// IsDirty reports whether the entry for id is absent or its hash differs from currentHash.
func (m *Manifest) IsDirty(id string, currentHash string) bool {
	entry, exists := m.Get(id)
	return !exists || entry.Hash != currentHash
}

// CleanStale removes entries whose IDs are not present in currentIDs and
// returns the removed entries so callers can delete the corresponding output
// files.
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
