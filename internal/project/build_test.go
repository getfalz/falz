package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBuild_SimpleMarkdownFile(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Hello World\n\nThis is a test.").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	stats, err := proj.Build(ctx)

	AssertNoError(t, err)

	if stats.TotalEntries == 0 {
		t.Error("Expected at least one entry")
	}
	if stats.BuiltEntries == 0 {
		t.Error("Expected at least one built entry")
	}

	AssertFileExists(t, proj.Config.OutputPath, "index.html")
}

func TestBuild_IncrementalBuild_NoChanges(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	_, err := proj.Build(ctx)
	AssertNoError(t, err)

	stats2, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats2.DirtyEntries != 0 {
		t.Errorf("Expected 0 dirty entries on second build, got %d", stats2.DirtyEntries)
	}
	if stats2.BuiltEntries != 0 {
		t.Errorf("Expected 0 built entries on second build, got %d", stats2.BuiltEntries)
	}
}

func TestBuild_ForceRebuild(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
			c.Build.ForceRebuild = true
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	_, err := proj.Build(ctx)
	AssertNoError(t, err)

	stats, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats.BuiltEntries == 0 {
		t.Error("Expected entries to be rebuilt with ForceRebuild")
	}
}

func TestBuild_ConcurrentBuild(t *testing.T) {
	proj := NewTestProject(t).
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 4
		})

	for i := range 10 {
		proj.WithSource(fmt.Sprintf("page%d.md", i), "# Page")
	}

	p, cleanup := proj.Build()
	defer cleanup()

	ctx := context.Background()
	stats, err := p.Build(ctx)

	AssertNoError(t, err)

	if stats.BuiltEntries != 10 {
		t.Errorf("Expected 10 built entries, got %d", stats.BuiltEntries)
	}
}

func TestBuild_CleanStaleEntries(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("old.md", "# Old").
		WithSource("new.md", "# New").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
			c.Build.CleanStale = true
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	_, err := proj.Build(ctx)
	AssertNoError(t, err)

	oldPath := filepath.Join(proj.Config.SourcePath, "old.md")
	os.Remove(oldPath)

	_, err = proj.Build(ctx)
	AssertNoError(t, err)

	oldOutput := filepath.Join(proj.Config.OutputPath, "old.html")
	if _, err := os.Stat(oldOutput); !os.IsNotExist(err) {
		t.Error("Expected old.html to be removed")
	}
}

func TestBuild_ContentAddressableStorage(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
			c.Build.UseCAS = true
			c.Build.CASPath = "cas"
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	stats1, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats1.CacheMisses == 0 {
		t.Error("Expected cache misses on first build")
	}

	os.RemoveAll(proj.Config.OutputPath)
	os.MkdirAll(proj.Config.OutputPath, 0755)

	stats2, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats2.CacheHits == 0 {
		t.Error("Expected cache hits on second build")
	}
}

func TestBuild_MultipleLayouts(t *testing.T) {
	layout1 := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template1.html">
	<Content name="content" src="*.md" pipe="md" />
</Layout>`

	layout2 := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template2.html">
	<Content name="content" src="*.md" pipe="md" />
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithSource("blog/post1.md", "# Blog Post").
		WithSource("docs/guide.md", "# Guide").
		WithScheme("blog/layout.xml", layout1).
		WithScheme("docs/layout.xml", layout2).
		WithScheme("blog/template1.html", "<div class='blog'>{{content}}</div>").
		WithScheme("docs/template2.html", "<div class='docs'>{{content}}</div>").
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	stats, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats.BuiltEntries != 2 {
		t.Errorf("Expected 2 built entries, got %d", stats.BuiltEntries)
	}

	AssertFileExists(t, proj.Config.OutputPath, "blog/post1.html")
	AssertFileExists(t, proj.Config.OutputPath, "docs/guide.html")
}

func TestBuild_ContextCancellation(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := proj.Build(ctx)

	if err == nil || !errors.Is(err, context.Canceled) {
		t.Error("Expected context.Canceled error")
	}
}

func TestBuild_BuildStats(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	stats, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats.Duration == 0 {
		t.Error("Expected non-zero duration")
	}
	if stats.PlanDuration == 0 {
		t.Error("Expected non-zero plan duration")
	}
	if stats.BuildDuration == 0 {
		t.Error("Expected non-zero build duration")
	}
	if stats.BytesWritten == 0 {
		t.Error("Expected bytes written")
	}
}
