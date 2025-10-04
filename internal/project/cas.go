package project

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CAS implements a content-addressable storage system
type CAS struct {
	root string
}

// NewCAS creates a new content-addressable storage
func NewCAS(root string) (*CAS, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create CAS root: %w", err)
	}

	return &CAS{
		root: root,
	}, nil
}

// keyPath returns the filesystem path for a given key
func (c *CAS) keyPath(key string) string {
	if len(key) < 4 {
		return filepath.Join(c.root, "short", key)
	}
	return filepath.Join(c.root, key[:2], key[2:4], key)
}

// Has checks if a key exists in the cache
func (c *CAS) Has(key string) bool {
	path := c.keyPath(key)
	_, err := os.Stat(path)

	return err == nil
}

// Store saves data with the given key
func (c *CAS) Store(key string, data []byte) error {
	path := c.keyPath(key)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create CAS dir: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write CAS temp: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename CAS file: %w", err)
	}

	return nil
}

// Restore retrieves data for the given key
func (c *CAS) Restore(key string) ([]byte, error) {
	path := c.keyPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CAS file: %w", err)
	}

	return data, nil
}

// Link creates a hard link or copies from cache to output path
func (c *CAS) Link(key string, outputPath string) error {
	src := c.keyPath(key)

	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("CAS key not found: %s", key)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	os.Remove(outputPath)

	if err := os.Link(src, outputPath); err == nil {
		return nil
	}

	return c.copyFile(src, outputPath, key)
}

// copyFile copies a file from src to dst
func (c *CAS) copyFile(src, dst, key string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("sync dest: %w", err)
	}

	return nil
}

// Size returns the total size of the cache in bytes
func (c *CAS) Size() (int64, error) {
	var total int64

	err := filepath.Walk(c.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})

	return total, err
}
