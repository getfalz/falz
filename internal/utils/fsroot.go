package utils

import (
	"craeft/internal/log"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"lukechampine.com/blake3"
)

type RootFS struct {
	root *os.Root
	hash *HashCache
}

// NewRootFs creates a new RootFs at the given path. The path is created if it does not exist.
func NewRootFs(path string) (*RootFS, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("output abs: %w", err)
	}
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return nil, fmt.Errorf("creating path: %w", err)
	}

	root, err := os.OpenRoot(absPath)
	if err != nil {
		return nil, fmt.Errorf("open root: %w", err)
	}

	return &RootFS{
		root: root,
		hash: NewHashCache(),
	}, nil
}

func (r *RootFS) FS() fs.FS {
	return r.root.FS()
}

func (r *RootFS) Glob(pattern string) ([]string, error) {
	return fs.Glob(r.root.FS(), pattern)
}

func (r *RootFS) HashFile(path string) (string, error) {
	return r.hash.HashFile(r.root.FS(), path)
}

func (r *RootFS) Close() error {
	return r.root.Close()
}

func (r *RootFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(r.root.FS(), name)
}

func (r *RootFS) Stat(name string) (os.FileInfo, error) {
	return r.root.Stat(name)
}

func (r *RootFS) Remove(name string) error {
	return r.root.Remove(name)
}

func (r *RootFS) Path() string {
	return r.root.Name()
}

// FindFile searches for files matching the given glob pattern by walking up the directory
// tree, starting from the combined basePath and targetPath, and stopping at basePath.
// It returns a slice of relative file paths (relative to basePath) for each unique match found.
//
// For example, if pattern is "*.config" and targetPath is "src/app", the function will look for
// matching files in:
//
//	basePath/src/app
//	basePath/src
//	basePath
//
// The search stops once basePath is reached. If no matches are found, an error is returned.
func (r *RootFS) FindFile(targetPath, pattern string) ([]string, error) {
	cur := filepath.ToSlash(strings.TrimPrefix(targetPath, "/"))
	if cur == "" {
		cur = "."
	}

	var results []string
	seen := make(map[string]struct{})
	for {
		glob := pattern
		if cur != "." {
			glob = path.Join(cur, pattern)
		}

		matches, err := r.Glob(glob)
		if err != nil {
			// fs.Glob only errors on malformed patterns
			return nil, fmt.Errorf("glob %q: %w", glob, err)
		}

		for _, fileName := range matches {
			if _, ok := seen[fileName]; !ok {
				results = append(results, fileName)
				seen[fileName] = struct{}{}
			}
		}

		if cur == "." {
			break
		}

		next := path.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no files matching %q found while walking up from %q", pattern, targetPath)
	}

	return results, nil
}

func (r *RootFS) WriteFile(dst string, data []byte, perm fs.FileMode) error {
	dst = filepath.Clean(filepath.ToSlash(dst))
	log.TTrace(log.IO).Str("dst", dst).Int("bytes", len(data)).Msg("writeFile begin")

	dir := filepath.Dir(dst)
	if err := r.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", dir, err)
	}

	tmpDst := dst + ".tmp"

	f, err := r.root.OpenFile(tmpDst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("open temp %q: %w", tmpDst, err)
	}

	success := false
	defer func() {
		if !success {
			f.Close()
			_ = r.root.Remove(tmpDst)
		}
	}()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %q: %w", tmpDst, err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync %q: %w", tmpDst, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close %q: %w", tmpDst, err)
	}

	if err := r.root.Rename(tmpDst, dst); err != nil {
		return fmt.Errorf("rename %q to %q: %w", tmpDst, dst, err)
	}

	success = true
	log.TTrace(log.IO).Str("dst", dst).Uint32("perm", uint32(perm)).Msg("writeFile done")
	return nil
}

func (r *RootFS) MkdirAll(path string, perm fs.FileMode) error {
	path = filepath.Clean(filepath.ToSlash(path))

	if filepath.IsAbs(path) {
		return fmt.Errorf("mkdir: absolute path not allowed: %s", path)
	}
	if vol := filepath.VolumeName(path); vol != "" {
		return fmt.Errorf("mkdir: volume name not allowed: %s", path)
	}

	if path == "." || path == "" {
		return nil
	}

	current := ""
	for part := range strings.SplitSeq(path, "/") {
		if part == "" || part == "." {
			continue
		}

		if current == "" {
			current = part
		} else {
			current = current + "/" + part
		}

		if err := r.root.Mkdir(current, perm); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("mkdir %s: %w", current, err)
		}
	}

	return nil
}

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

// HashFile computes the hash of a file, returning cached hash if unchanged
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

	// TODO deprecate this double-checking logic
	// check if file changed during hashing
	// info2, err := fs.Stat(fsys, path)
	// if err != nil {
	// 	return "", fmt.Errorf("re-stat %s: %w", path, err)
	// }
	//
	// if !info.ModTime().Equal(info2.ModTime()) || info.Size() != info2.Size() {
	// 	return h.HashFile(fsys, path)
	// }

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

// hashBytes computes the hash of byte data
func hashBytes(data []byte) string {
	hasher := blake3.New(32, nil)
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}

// HashString computes the hash of a string
func HashString(s string) string {
	return hashBytes([]byte(s))
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
