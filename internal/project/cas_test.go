package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestPlan_BasicPlan(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	if plan.LenEntries() == 0 {
		t.Error("Expected at least one entry in plan")
	}

	keys := plan.EntryKeys(func(e *BuildEntry) bool {
		return e.ID == "index.md"
	})
	if len(keys) != 1 {
		t.Error("Expected to find index.md in plan")
	}
}

func TestPlan_GetDirtyEntries(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	dirty := plan.GetDirtyEntries()

	if len(dirty) != plan.LenEntries() {
		t.Errorf("Expected all %d entries to be dirty, got %d",
			plan.LenEntries(), len(dirty))
	}
}

func TestPlan_GetEntriesByType(t *testing.T) {
	assetLayout := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="layout.html">
	<Content name="content" src="*.md" pipe="md">
		<Asset name="style" src="*.css" pipe="copy" />
	</Content>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithSource("style.css", "body { color: red; }").
		WithScheme("layout.xml", assetLayout).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: css
target_suffix: css`,
			"index.js": `function transform(ctx, input) { return input; }`,
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	contentEntries := plan.GetEntriesByType("content")
	assetEntries := plan.GetEntriesByType("asset")

	if len(contentEntries) == 0 {
		t.Error("Expected content entries")
	}
	if len(assetEntries) == 0 {
		t.Error("Expected asset entries")
	}
}

func TestPlan_HashComputation(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	hashKeys := plan.EntryKeys(func(e *BuildEntry) bool {
		return e.Hash == ""
	})
	if len(hashKeys) != 0 {
		t.Error("Some entries have empty hash")
	}
	contentKeys := plan.EntryKeys(func(e *BuildEntry) bool {
		return e.ContentHash == ""
	})
	if len(contentKeys) != 0 {
		t.Error("Some entries have empty content hash")
	}
}

func TestPlan_MultipleFiles(t *testing.T) {
	proj := NewTestProject(t).
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline())

	// Add multiple source files
	for i := 0; i < 10; i++ {
		filename := "page" + string(rune('0'+i)) + ".md"
		proj.WithSource(filename, "# Page")
	}

	p, cleanup := proj.Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := p.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	if plan.LenEntries() != 10 {
		t.Errorf("Expected 10 entries, got %d", plan.LenEntries())
	}
}

func TestPlan_NestedDirectories(t *testing.T) {
	layout := `<?xml version="1.0" encoding="UTF-8"?>
<Layout tpl="layout.html">
	<Content name="content" src="**/*.md" pipe="md" />
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithSource("a/b/c/deep.md", "# Deep").
		WithSource("a/shallow.md", "# Shallow").
		WithScheme("layout.xml", layout).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Should find both files
	if plan.LenEntries() < 2 {
		t.Errorf("Expected at least 2 entries, got %d", plan.LenEntries())
	}
}

func TestPlan_IgnorePatterns(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("include.md", "# Include").
		WithSource("node_modules/excluded.md", "# Exclude").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Ignore = append(c.Ignore, "node_modules/**")
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	keys := plan.EntryKeys(func(e *BuildEntry) bool {
		return strings.Contains(e.ID, "node_modules")

	})

	if len(keys) != 0 {
		t.Error("Expected node_modules to be ignored")
	}
}
