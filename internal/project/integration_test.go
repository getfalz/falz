package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegration_CompleteWorkflow(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" />
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html">
		<Asset name="styles" src="*.css" to="{FILENAME}.{EXT}"/>
	</Content>
	<Fragment name="footer" src="footer.html" />
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Welcome\n\nHome page").
		WithSource("about.md", "# About\n\nAbout us").
		WithSource("style.css", "body { margin: 0; }").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "{{header}}<main>{{content}}</main>{{footer}}").
		WithScheme("header.html", "<header>My Site</header>").
		WithScheme("footer.html", "<footer>&copy; 2025</footer>").
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 2
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	stats, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats.BuiltEntries == 0 {
		t.Fatal("Expected files to be built")
	}

	AssertFileExists(t, proj.Config.OutputPath, "index.html")
	AssertFileExists(t, proj.Config.OutputPath, "about.html")
	AssertFileExists(t, proj.Config.OutputPath, "style.css")

	indexContent, _ := os.ReadFile(filepath.Join(proj.Config.OutputPath, "index.html"))
	if !strings.Contains(string(indexContent), "My Site") {
		t.Error("Expected header fragment in output")
	}
	if !strings.Contains(string(indexContent), "Welcome") {
		t.Error("Expected content in output")
	}

	stats2, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats2.DirtyEntries != 0 {
		t.Errorf("Expected 0 dirty entries, got %d", stats2.DirtyEntries)
	}
}

func TestIntegration_BlogSite(t *testing.T) {
	blogLayout := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Content name="post" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithSource("blog/posts/post1.md", "# Post 1").
		WithSource("blog/posts/post2.md", "# Post 2").
		WithSource("blog/posts/post3.md", "# Post 3").
		WithScheme("blog/layout.xml", blogLayout).
		WithScheme("blog/template.html", "<article>{{post}}</article>").
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 2
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	stats, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats.BuiltEntries != 3 {
		t.Errorf("Expected 3 blog posts, got %d", stats.BuiltEntries)
	}

	AssertFileExists(t, proj.Config.OutputPath, "blog/posts/post1.html")
	AssertFileExists(t, proj.Config.OutputPath, "blog/posts/post2.html")
	AssertFileExists(t, proj.Config.OutputPath, "blog/posts/post3.html")
}

func TestIntegration_ChangeDetection(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Version 1").
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

	err = os.WriteFile(
		filepath.Join(proj.Config.SourcePath, "index.md"),
		[]byte("# Version 2"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to update file: %v", err)
	}

	stats2, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats2.DirtyEntries != 1 {
		t.Errorf("Expected 1 dirty entry after change, got %d", stats2.DirtyEntries)
	}

	content, _ := os.ReadFile(filepath.Join(proj.Config.OutputPath, "index.html"))
	if !strings.Contains(string(content), "Version 2") {
		t.Error("Expected updated content in output")
	}
}

func TestIntegration_ErrorHandling(t *testing.T) {
	badPipeline := map[string]string{
		"config.yaml": `name: bad
entry_point: index.js
source_suffix: md
target_suffix: html`,
		"index.js": `function transform(input) {
	throw new Error("Pipeline error");
}`,
	}

	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="layout.html">
	<Content name="content" src="*.md" pipe="bad" to="{FILENAME}.html"/>
</Layout>`).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("bad", badPipeline).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	stats, err := proj.Build(ctx)

	if stats == nil {
		t.Fatal("Expected stats even with errors")
	}

	if len(stats.Errors) == 0 {
		t.Error("Expected errors from bad pipeline")
	}

	if err == nil {
		t.Error("Expected error from build")
	}
}

func TestIntegration_CleanBuild(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("keep.md", "# Keep").
		WithSource("remove.md", "# Remove").
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

	AssertFileExists(t, proj.Config.OutputPath, "keep.html")
	AssertFileExists(t, proj.Config.OutputPath, "remove.html")

	os.Remove(filepath.Join(proj.Config.SourcePath, "remove.md"))

	_, err = proj.Build(ctx)
	AssertNoError(t, err)

	removePath := filepath.Join(proj.Config.OutputPath, "remove.html")
	if _, err := os.Stat(removePath); !os.IsNotExist(err) {
		t.Error("Expected remove.html to be deleted")
	}

	AssertFileExists(t, proj.Config.OutputPath, "keep.html")
}

func TestIntegration_CASReuse(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test Content").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
			c.SetCasPath("./cas")
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	stats1, err := proj.Build(ctx)
	AssertNoError(t, err)

	cacheSize1 := stats1.BytesCached

	os.RemoveAll(proj.Config.OutputPath)
	os.MkdirAll(proj.Config.OutputPath, 0755)

	// Build again - should restore from CAS
	stats2, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats2.CacheHits == 0 {
		t.Error("Expected cache hits when rebuilding from CAS")
	}

	AssertFileExists(t, proj.Config.OutputPath, "index.html")

	if stats2.BytesCached > cacheSize1 {
		t.Logf("Cache grew from %d to %d bytes", cacheSize1, stats2.BytesCached)
	}
}
