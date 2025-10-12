package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCAS_StoreAndRestore(t *testing.T) {
	tempDir := t.TempDir()
	cas, err := NewCAS(tempDir)
	if err != nil {
		t.Fatalf("Failed to create CAS: %v", err)
	}

	data := []byte("test data")
	key := HashBytes(data)

	err = cas.Store(key, data)
	if err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	restored, err := cas.Restore(key)
	if err != nil {
		t.Fatalf("Failed to restore: %v", err)
	}

	if string(restored) != string(data) {
		t.Errorf("Expected %s, got %s", string(data), string(restored))
	}
}

func TestCAS_Has(t *testing.T) {
	tempDir := t.TempDir()
	cas, err := NewCAS(tempDir)
	if err != nil {
		t.Fatalf("Failed to create CAS: %v", err)
	}

	data := []byte("test")
	key := HashBytes(data)

	if cas.Has(key) {
		t.Error("Expected key to not exist")
	}

	cas.Store(key, data)

	if !cas.Has(key) {
		t.Error("Expected key to exist")
	}
}

func TestCAS_Link(t *testing.T) {
	tempDir := t.TempDir()
	cas, err := NewCAS(tempDir)
	if err != nil {
		t.Fatalf("Failed to create CAS: %v", err)
	}

	data := []byte("test data")
	key := HashBytes(data)

	err = cas.Store(key, data)
	if err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	outputPath := filepath.Join(tempDir, "output", "test.txt")
	err = cas.Link(key, outputPath)
	if err != nil {
		t.Fatalf("Failed to link: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read linked file: %v", err)
	}

	if string(content) != string(data) {
		t.Errorf("Expected %s, got %s", string(data), string(content))
	}
}

func TestCAS_Size(t *testing.T) {
	tempDir := t.TempDir()
	cas, err := NewCAS(tempDir)
	if err != nil {
		t.Fatalf("Failed to create CAS: %v", err)
	}

	for i := range 5 {
		data := []byte("test data " + string(rune('0'+i)))
		key := HashBytes(data)
		cas.Store(key, data)
	}

	size, err := cas.Size()
	if err != nil {
		t.Fatalf("Failed to get size: %v", err)
	}

	if size == 0 {
		t.Error("Expected non-zero size")
	}
}

func TestCAS_ShortKey(t *testing.T) {
	tempDir := t.TempDir()
	cas, err := NewCAS(tempDir)
	if err != nil {
		t.Fatalf("Failed to create CAS: %v", err)
	}

	data := []byte("test")
	shortKey := "abc"

	err = cas.Store(shortKey, data)
	if err != nil {
		t.Fatalf("Failed to store with short key: %v", err)
	}

	restored, err := cas.Restore(shortKey)
	if err != nil {
		t.Fatalf("Failed to restore with short key: %v", err)
	}

	if string(restored) != string(data) {
		t.Error("Short key storage/retrieval failed")
	}
}
