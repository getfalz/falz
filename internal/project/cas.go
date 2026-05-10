package project

import (
	"craeft/internal/log"
	"craeft/internal/utils"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CAS implements a content-addressable storage system.
type CAS struct {
	rootFS   *utils.RootFS
	outputFS *utils.RootFS
	absRoot  string
}

// NewCAS creates a new content-addressable storage
func NewCAS(path string, outputFS *utils.RootFS) (*CAS, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("cas abs path: %w", err)
	}

	// Reject symlinks at the root to prevent escaping the sandbox.
	if fi, err := os.Lstat(absPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("cas base is a symlink: %s", absPath)
	}

	rootFS, err := utils.NewRootFs(absPath)
	if err != nil {
		return nil, fmt.Errorf("create CAS rootFS: %w", err)
	}

	log.IInfo(log.CAS).
		Str("path", absPath).
		Msg("CAS enabled")

	return &CAS{
		rootFS:   rootFS,
		outputFS: outputFS,
		absRoot:  absPath,
	}, nil
}

// Close releases the underlying os.Root handle inside the RootFS.
func (c *CAS) Close() error {
	return c.rootFS.Close()
}

// keyRel returns the relative path inside the CAS root for a given key.
func (c *CAS) keyRel(key string) string {
	if len(key) < 4 {
		return "short/" + key
	}
	return key[:2] + "/" + key[2:4] + "/" + key
}

func (c *CAS) keyAbs(key string) string {
	return filepath.Join(c.absRoot, filepath.FromSlash(c.keyRel(key)))
}

// Has reports whether key exists in the cache.
func (c *CAS) Has(key string) bool {
	_, err := c.rootFS.Stat(c.keyRel(key))
	return err == nil
}

// Store saves data with the given key
func (c *CAS) Store(key string, data []byte) error {
	rel := c.keyRel(key)
	if err := c.rootFS.WriteFile(rel, data, 0o600); err != nil {
		return fmt.Errorf("store CAS key %s: %w", key, err)
	}
	return nil
}

// Link creates a hard link or copies from cache to output path
func (c *CAS) Link(key string, outputPath string) error {
	if !c.Has(key) {
		return fmt.Errorf("CAS key not found: %s", key)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	src := c.keyAbs(key)
	absOutputPath, err := filepath.Abs(filepath.Join(c.outputFS.Path(), outputPath))
	if err != nil {
		return fmt.Errorf("output abs: %w", err)
	}

	_ = os.Remove(absOutputPath)

	err = os.Link(src, absOutputPath)
	if err == nil {
		log.TTrace(log.CAS).
			Str("key", c.keyRel(key)).
			Str("output", outputPath).
			Msg("CAS hard link")
		return nil
	}

	log.DDebug(log.CAS).
		Str("key", c.keyRel(key)).
		Str("output", outputPath).
		Err(err).
		Msg("CAS hard link failed, copying")
	return c.copyOut(key, absOutputPath)
}

// copyOut copies a file with key to outputPath
func (c *CAS) copyOut(key, outputPath string) error {
	src, err := c.rootFS.FS().Open(c.keyRel(key))
	if err != nil {
		return fmt.Errorf("open CAS source %q: %w", key, err)
	}
	defer src.Close()

	dst, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create dest %q: %w", outputPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy CAS -> %q: %w", outputPath, err)
	}
	if err := dst.Sync(); err != nil {
		return fmt.Errorf("sync dest %q: %w", outputPath, err)
	}
	return nil
}
