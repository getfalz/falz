package project

import (
	"context"
	"strings"
	"testing"
)

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
	for i := range 10 {
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
