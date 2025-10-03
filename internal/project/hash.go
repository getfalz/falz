package project

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"sync"
	"time"

	"lukechampine.com/blake3"
)

// HashCache caches file hashes based on mtime and size
type HashCache struct {
	mu    sync.RWMutex
	cache map[string]hashEntry
}

type hashEntry struct {
	hash  string
	mtime time.Time
	size  int64
}

func NewHashCache() *HashCache {
	return &HashCache{
		cache: make(map[string]hashEntry),
	}
}

// HashFile computes the hash of a file, using cache if unchanged
func (h *HashCache) HashFile(fsys fs.FS, path string) (string, error) {
	info, err := fs.Stat(fsys, path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}

	h.mu.RLock()
	entry, exists := h.cache[path]
	h.mu.RUnlock()

	if exists && entry.mtime.Equal(info.ModTime()) && entry.size == info.Size() {
		return entry.hash, nil
	}

	hash, err := h.computeFileHash(fsys, path)
	if err != nil {
		return "", err
	}

	info2, err := fs.Stat(fsys, path)
	if err != nil {
		return "", fmt.Errorf("re-stat %s: %w", path, err)
	}

	if !info.ModTime().Equal(info2.ModTime()) || info.Size() != info2.Size() {
		return h.HashFile(fsys, path)
	}

	h.mu.Lock()
	h.cache[path] = hashEntry{
		hash:  hash,
		mtime: info.ModTime(),
		size:  info.Size(),
	}
	h.mu.Unlock()

	return hash, nil
}

func (h *HashCache) computeFileHash(fsys fs.FS, path string) (string, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	hasher := blake3.New(32, nil)
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// HashBytes computes the hash of byte data
func HashBytes(data []byte) string {
	hasher := blake3.New(32, nil)
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}

// HashString computes the hash of a string
func HashString(s string) string {
	return HashBytes([]byte(s))
}

// CombineHashes combines multiple hashes into a single hash
func CombineHashes(hashes ...string) string {
	sorted := make([]string, len(hashes))
	copy(sorted, hashes)
	sort.Strings(sorted)

	hasher := blake3.New(32, nil)
	for _, h := range sorted {
		hasher.Write([]byte(h))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// EntryHash represents all the components that make up an entry's hash
type EntryHash struct {
	ContentHash    string
	LayoutHash     string
	PipelineHash   string
	FragmentHashes map[string]string // fragment path -> hash
}

// Compute returns the combined hash for the entry
func (e *EntryHash) Compute() string {
	var fragmentHashes []string
	for _, h := range e.FragmentHashes {
		fragmentHashes = append(fragmentHashes, h)
	}

	return CombineHashes(
		e.ContentHash,
		e.LayoutHash,
		e.PipelineHash,
		CombineHashes(fragmentHashes...),
	)
}
