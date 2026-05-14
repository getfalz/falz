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
	casFS    *utils.RootFS
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
		casFS:    rootFS,
		outputFS: outputFS,
		absRoot:  absPath,
	}, nil
}

// Close releases the underlying os.Root handle inside the RootFS.
func (c *CAS) Close() error {
	return c.casFS.Close()
}

// keyRel returns the relative path inside the CAS root for a given key.
func (c *CAS) keyRel(key string) string {
	if len(key) < 4 {
		return "short/" + key
	}
	return key[:2] + "/" + key[2:4] + "/" + key
}

// Has reports whether key exists in the cache.
func (c *CAS) Has(key string) bool {
	_, err := c.casFS.Stat(c.keyRel(key))
	return err == nil
}

// Store saves data with the given key
func (c *CAS) Store(key string, data []byte) error {
	rel := c.keyRel(key)

	if err := c.casFS.MkdirAll(filepath.Dir(rel), CacheDirPerm); err != nil {
		return fmt.Errorf("create CAS dir for key %s: %w", key, err)
	}

	if err := c.casFS.WriteFile(rel, data, CacheFilePerm); err != nil {
		return fmt.Errorf("store CAS key %s: %w", key, err)
	}
	return nil
}

// Restore retrieves the data for a given key and writes it to outputPath
func (c *CAS) Restore(key string, outputPath string) error {
	if !c.Has(key) {
		return fmt.Errorf("CAS key not found: %s", key)
	}

	// if err := os.MkdirAll(filepath.Dir(outputPath), OutputDirPerm); err != nil {
	// 	return fmt.Errorf("create output dir: %w", err)
	// }

	absOutputPath, err := filepath.Abs(filepath.Join(c.outputFS.Path(), outputPath))
	if err != nil {
		return fmt.Errorf("output abs: %w", err)
	}

	log.TTrace(log.CAS).
		Str("key", c.keyRel(key)).
		Str("output", outputPath).
		Msg("CAS restore")
	return c.copyOut(key, absOutputPath)
}

// copyOut copies a file with key to outputPath
func (c *CAS) copyOut(key, outputPath string) error {
	src, err := c.casFS.FS().Open(c.keyRel(key))
	if err != nil {
		return fmt.Errorf("open CAS source %q: %w", key, err)
	}
	defer src.Close()

	dst, err := os.OpenFile(
		outputPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		OutputFilePerm,
	)
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
