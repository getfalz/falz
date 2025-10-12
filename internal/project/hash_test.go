package project

import (
	"testing"
	"testing/fstest"
	"time"
)

func TestHash_Cache_FileHashing(t *testing.T) {
	cache := NewHashCache()

	fs := fstest.MapFS{
		"test.txt": &fstest.MapFile{
			Data:    []byte("hello world"),
			ModTime: time.Now(),
		},
	}

	hash1, err := cache.HashFile(fs, "test.txt")
	if err != nil {
		t.Fatalf("Failed to hash file: %v", err)
	}

	// Hash again - should use cache
	hash2, err := cache.HashFile(fs, "test.txt")
	if err != nil {
		t.Fatalf("Failed to hash file second time: %v", err)
	}

	if hash1 != hash2 {
		t.Error("Expected same hash from cache")
	}
}

func TestHash_Cache_InvalidatesOnChange(t *testing.T) {
	cache := NewHashCache()

	modTime1 := time.Now()
	fs1 := fstest.MapFS{
		"test.txt": &fstest.MapFile{
			Data:    []byte("hello"),
			ModTime: modTime1,
		},
	}

	hash1, err := cache.HashFile(fs1, "test.txt")
	if err != nil {
		t.Fatalf("Failed to hash file: %v", err)
	}

	// Simulate file change
	modTime2 := modTime1.Add(time.Second)
	fs2 := fstest.MapFS{
		"test.txt": &fstest.MapFile{
			Data:    []byte("hello world"),
			ModTime: modTime2,
		},
	}

	hash2, err := cache.HashFile(fs2, "test.txt")
	if err != nil {
		t.Fatalf("Failed to hash changed file: %v", err)
	}

	if hash1 == hash2 {
		t.Error("Expected different hash after file change")
	}
}

func TestHash_Bytes(t *testing.T) {
	data := []byte("test data")
	hash1 := HashBytes(data)
	hash2 := HashBytes(data)

	if hash1 != hash2 {
		t.Error("Expected consistent hashing")
	}

	if len(hash1) != 64 { // blake3 produces 32 bytes = 64 hex chars
		t.Errorf("Expected 64 character hash, got %d", len(hash1))
	}
}

func TestHash_String(t *testing.T) {
	str := "test string"
	hash1 := HashString(str)
	hash2 := HashString(str)

	if hash1 != hash2 {
		t.Error("Expected consistent hashing")
	}

	differentHash := HashString("different")
	if hash1 == differentHash {
		t.Error("Expected different hashes for different strings")
	}
}

func TestHash_CombineHashes(t *testing.T) {
	hash1 := HashString("a")
	hash2 := HashString("b")
	hash3 := HashString("c")

	combined1 := CombineHashes(hash1, hash2, hash3)
	combined2 := CombineHashes(hash1, hash2, hash3)

	if combined1 != combined2 {
		t.Error("Expected consistent combined hash")
	}

	// Order shouldn't matter (hashes are sorted)
	combined3 := CombineHashes(hash3, hash1, hash2)
	if combined1 != combined3 {
		t.Error("Expected same combined hash regardless of order")
	}
}

func TestHash_EntryHash_Compute(t *testing.T) {
	entryHash := EntryHash{
		ContentHash:  HashString("content"),
		LayoutHash:   HashString("layout"),
		PipelineHash: HashString("pipeline"),
		FragmentHashes: map[string]string{
			"frag1": HashString("fragment1"),
			"frag2": HashString("fragment2"),
		},
	}

	hash1 := entryHash.Compute()
	hash2 := entryHash.Compute()

	if hash1 != hash2 {
		t.Error("Expected consistent entry hash computation")
	}

	// Change content
	entryHash.ContentHash = HashString("different")
	hash3 := entryHash.Compute()

	if hash1 == hash3 {
		t.Error("Expected different hash after content change")
	}
}

func TestHash_EntryHash_EmptyFragments(t *testing.T) {
	entryHash := EntryHash{
		ContentHash:    HashString("content"),
		LayoutHash:     "",
		PipelineHash:   HashString("pipeline"),
		FragmentHashes: map[string]string{},
	}

	hash := entryHash.Compute()
	if hash == "" {
		t.Error("Expected non-empty hash even with empty fragments")
	}
}
