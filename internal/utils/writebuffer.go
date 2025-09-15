package utils

import (
	"craeft/internal/log"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type WriteBuffer struct {
	root    *os.Root
	mu      sync.Mutex
	pending map[string]BufferedWrite
	maxSize int64

	totalSize    int64
	flushCount   int
	pendingCount int
}

// BufferedWrite represents a pending write operation
type BufferedWrite struct {
	Data []byte
	Perm fs.FileMode
}

// WriteBufferConfig configures the write buffer behavior
type WriteBufferConfig struct {
	// MaxSize is the maximum buffer size before auto-flush (in bytes)
	MaxSize int64
}

func NewWriteBuffer(root *os.Root, config WriteBufferConfig) *WriteBuffer {
	return &WriteBuffer{
		root:    root,
		pending: make(map[string]BufferedWrite),
		maxSize: config.MaxSize,
	}
}

// Add buffers a file write operation
func (wb *WriteBuffer) Add(dst string, data []byte, perm fs.FileMode) error {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	dst = filepath.Clean(filepath.ToSlash(dst))

	wb.pending[dst] = BufferedWrite{
		Data: data,
		Perm: perm,
	}

	wb.totalSize += int64(len(data))
	wb.pendingCount++

	log.TTrace(log.IO).
		Str("dst", dst).
		Int("bytes", len(data)).
		Int64("total_buffered", wb.totalSize).
		Int("pending_files", wb.pendingCount).
		Msg("added to write buffer")

	if wb.totalSize > wb.maxSize {
		return wb.flushUnsafe()
	}

	return nil
}

func (wb *WriteBuffer) Flush() error {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	return wb.flushUnsafe()
}

func (wb *WriteBuffer) flushUnsafe() error {
	if len(wb.pending) == 0 {
		return nil
	}

	wb.flushCount++
	flushNum := wb.flushCount
	pendingCount := wb.pendingCount
	totalSize := wb.totalSize

	log.IInfo(log.IO).
		Int("flush_num", flushNum).
		Int("files", pendingCount).
		Int64("total_bytes", totalSize).
		Msg("write buffer flush started")

	dirGroups := wb.groupByDirectory()

	var errors []error
	filesWritten := 0
	bytesWritten := int64(0)

	for dir, writes := range dirGroups {
		dirFiles, dirBytes, err := wb.flushDirectory(dir, writes)
		filesWritten += dirFiles
		bytesWritten += dirBytes

		if err != nil {
			errors = append(errors, fmt.Errorf("flush directory %s: %w", dir, err))
		}
	}

	wb.pending = make(map[string]BufferedWrite)
	wb.totalSize = 0
	wb.pendingCount = 0

	log.IInfo(log.IO).
		Int("flush_num", flushNum).
		Int("files_written", filesWritten).
		Int64("bytes_written", bytesWritten).
		Int("directories", len(dirGroups)).
		Int("errors", len(errors)).
		Msg("write buffer flush completed")

	if len(errors) > 0 {
		return fmt.Errorf("write buffer flush failed: %v", errors)
	}

	return nil
}

// groupByDirectory groups pending writes by their target directory
func (wb *WriteBuffer) groupByDirectory() map[string]map[string]BufferedWrite {
	dirGroups := make(map[string]map[string]BufferedWrite)

	for path, write := range wb.pending {
		dir := filepath.Dir(path)
		if dirGroups[dir] == nil {
			dirGroups[dir] = make(map[string]BufferedWrite)
		}
		dirGroups[dir][path] = write
	}

	return dirGroups
}

// flushDirectory writes all files in a directory sequentially
func (wb *WriteBuffer) flushDirectory(dir string, writes map[string]BufferedWrite) (int, int64, error) {
	if len(writes) == 0 {
		return 0, 0, nil
	}

	log.DDebug(log.IO).
		Str("directory", dir).
		Int("files", len(writes)).
		Msg("flushing directory")

	// Ensure directory exists (only once per directory)
	if err := MkdirAllFS(wb.root, dir, 0o755); err != nil {
		return 0, 0, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	filesWritten := 0
	bytesWritten := int64(0)

	// Write files sequentially within the directory
	for path, write := range writes {
		if err := wb.writeFileBuffered(path, write.Data, write.Perm); err != nil {
			log.EError(log.IO).
				Str("path", path).
				Err(err).
				Msg("failed to write buffered file")
			// Continue with other files in best-effort mode
			continue
		}
		filesWritten++
		bytesWritten += int64(len(write.Data))
	}

	// Single sync per directory for better performance
	if filesWritten > 0 {
		if err := wb.syncDirectory(dir); err != nil {
			log.Warn(fmt.Sprintf("failed to sync directory %s: %v", dir, err))
			// Don't fail the entire operation for sync errors
		}
	}

	return filesWritten, bytesWritten, nil
}

// writeFileBuffered writes a single file (used internally by flush)
func (wb *WriteBuffer) writeFileBuffered(dst string, data []byte, perm fs.FileMode) error {
	dst = filepath.Clean(filepath.ToSlash(dst))

	log.TTrace(log.IO).
		Str("dst", dst).
		Int("bytes", len(data)).
		Msg("writing buffered file")

	f, err := wb.root.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open %q: %w", dst, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %q: %w", dst, err)
	}

	if err := f.Chmod(perm); err != nil {
		return fmt.Errorf("chmod %q: %w", dst, err)
	}

	// Note: We don't sync individual files here - we sync the directory once

	return nil
}

// syncDirectory syncs a directory for better I/O performance
func (wb *WriteBuffer) syncDirectory(dir string) error {
	// Open directory for syncing
	dirFile, err := wb.root.OpenFile(dir, os.O_RDONLY, 0)
	if err != nil {
		// Some filesystems don't support directory sync
		return nil
	}
	defer dirFile.Close()

	// Sync directory metadata
	if err := dirFile.Sync(); err != nil {
		// Some filesystems don't support directory sync
		return nil
	}

	return nil
}

// Stats returns buffer statistics
func (wb *WriteBuffer) Stats() WriteBufferStats {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	return WriteBufferStats{
		PendingFiles: wb.pendingCount,
		PendingBytes: wb.totalSize,
		FlushCount:   wb.flushCount,
	}
}

// WriteBufferStats contains buffer statistics
type WriteBufferStats struct {
	PendingFiles int
	PendingBytes int64
	FlushCount   int
}

// Close flushes any remaining buffered writes and cleans up
func (wb *WriteBuffer) Close() error {
	return wb.Flush()
}
