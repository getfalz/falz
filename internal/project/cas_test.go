package project

import (
	"os"
	"path/filepath"
	"testing"

	"craeft/internal/utils"
)

func newTestOutputRoot(t *testing.T) *utils.RootFS {
	t.Helper()
	root, err := utils.NewRootFs(t.TempDir())
	if err != nil {
		t.Fatalf("new output root fs: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func newTestCAS(t *testing.T) (*CAS, *utils.RootFS) {
	t.Helper()
	out := newTestOutputRoot(t)
	cas, err := NewCAS(filepath.Join(t.TempDir(), "cas"), out)
	if err != nil {
		t.Fatalf("NewCAS: %v", err)
	}
	t.Cleanup(func() { _ = cas.Close() })
	return cas, out
}

func TestCAS_Has_FalseUntilStored(t *testing.T) {
	cas, _ := newTestCAS(t)

	if cas.Has("some-hash") {
		t.Fatalf("expected Has to be false before Store")
	}
	if err := cas.Store("some-hash", []byte("data")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !cas.Has("some-hash") {
		t.Fatalf("expected Has to be true after Store")
	}
}

func TestCAS_Has_ShortKey(t *testing.T) {
	cas, _ := newTestCAS(t)

	if err := cas.Store("ab", []byte("short-key-data")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !cas.Has("ab") {
		t.Fatalf("expected Has to be true for a short key")
	}
}

func TestCAS_Store_DoesNotOverwriteExistingKey(t *testing.T) {
	cas, out := newTestCAS(t)

	if err := cas.Store("hash1", []byte("original")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	// Same key, different data: should be a no-op since the key already exists.
	if err := cas.Store("hash1", []byte("different")); err != nil {
		t.Fatalf("Store (2nd): %v", err)
	}

	if err := cas.Restore("hash1", "out.txt"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	data, err := out.ReadFile("out.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("expected the first stored value to survive, got %q", data)
	}
}

func TestCAS_Restore_CopiesStoredDataToOutputPath(t *testing.T) {
	cas, out := newTestCAS(t)

	if err := cas.Store("hash1", []byte("hello world")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if err := cas.Restore("hash1", "out.txt"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := out.ReadFile("out.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("got %q, want %q", data, "hello world")
	}
}

func TestCAS_Restore_MissingKey_ReturnsError(t *testing.T) {
	cas, _ := newTestCAS(t)

	if err := cas.Restore("does-not-exist", "out.txt"); err == nil {
		t.Fatalf("expected error restoring a key that was never stored")
	}
}

// TestCAS_Restore_RequiresOutputDirToAlreadyExist documents that Restore
// does not create missing intermediate directories in the output path
// (the MkdirAll call in Restore is currently commented out); callers must
// create the output directory first, as builder.writeArtifact does.
func TestCAS_Restore_RequiresOutputDirToAlreadyExist(t *testing.T) {
	cas, _ := newTestCAS(t)

	if err := cas.Store("hash1", []byte("data")); err != nil {
		t.Fatalf("Store: %v", err)
	}

	if err := cas.Restore("hash1", "nested/dir/out.txt"); err == nil {
		t.Fatalf("expected Restore to fail when the output directory doesn't exist yet")
	}
}

func TestNewCAS_RejectsSymlinkRoot(t *testing.T) {
	out := newTestOutputRoot(t)

	realDir := filepath.Join(t.TempDir(), "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	linkPath := filepath.Join(t.TempDir(), "cas-link")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, err := NewCAS(linkPath, out); err == nil {
		t.Fatalf("expected NewCAS to reject a symlinked root")
	}
}
