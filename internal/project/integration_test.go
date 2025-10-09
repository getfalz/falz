package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegration_CompleteWorkflow(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout tpl="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md">
		<Asset name="styles" src="*.css" pipe="copy" />
	</Content>
	<Fragment name="footer" src="footer.html" pipe="copy" />
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
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: "(html|css)"
target_suffix: "$1"`,
			"index.js": `function transform(ctx, input) { return input; }`,
		}).
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

	// Verify outputs
	AssertFileExists(t, proj.Config.OutputPath, "index.html")
	AssertFileExists(t, proj.Config.OutputPath, "about.html")
	AssertFileExists(t, proj.Config.OutputPath, "style.css")

	// Check content
	indexContent, _ := os.ReadFile(filepath.Join(proj.Config.OutputPath, "index.html"))
	if !strings.Contains(string(indexContent), "My Site") {
		t.Error("Expected header fragment in output")
	}
	if !strings.Contains(string(indexContent), "Welcome") {
		t.Error("Expected content in output")
	}

	// Incremental build (no changes)
	stats2, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats2.DirtyEntries != 0 {
		t.Errorf("Expected 0 dirty entries, got %d", stats2.DirtyEntries)
	}
}

func TestIntegration_BlogSite(t *testing.T) {
	blogLayout := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="blog/template.html">
	<Content name="post" src="posts/*.md" pipe="md" />
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

	// Modify source file
	time.Sleep(10 * time.Millisecond) // Ensure mtime changes
	err = os.WriteFile(
		filepath.Join(proj.Config.SourcePath, "index.md"),
		[]byte("# Version 2"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to update file: %v", err)
	}

	// Build again - should detect change
	stats2, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats2.DirtyEntries != 1 {
		t.Errorf("Expected 1 dirty entry after change, got %d", stats2.DirtyEntries)
	}

	// Verify new content
	content, _ := os.ReadFile(filepath.Join(proj.Config.OutputPath, "index.html"))
	if !strings.Contains(string(content), "Version 2") {
		t.Error("Expected updated content in output")
	}
}

func TestIntegration_LayoutChange(t *testing.T) {
	layoutV1 := `<?xml version="1.0" encoding="UTF-8"?>
<Layout tpl="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md" />
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", layoutV1).
		WithScheme("template.html", "{{header}}{{content}}").
		WithScheme("header.html", "<header>V1</header>").
		WithPipeline("md", BasicMarkdownPipeline()).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: html
target_suffix: html`,
			"index.js": `function transform(ctx, input) { return input; }`,
		}).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	// Initial build
	_, err := proj.Build(ctx)
	AssertNoError(t, err)

	content1, _ := os.ReadFile(filepath.Join(proj.Config.OutputPath, "index.html"))
	if !strings.Contains(string(content1), "V1") {
		t.Error("Expected V1 header")
	}

	// Modify fragment
	time.Sleep(10 * time.Millisecond)
	err = os.WriteFile(
		filepath.Join(proj.Config.SchemePath, "header.html"),
		[]byte("<header>V2</header>"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to update fragment: %v", err)
	}

	// Rebuild - should detect fragment change
	_, err = proj.Build(ctx)
	AssertNoError(t, err)

	content2, _ := os.ReadFile(filepath.Join(proj.Config.OutputPath, "index.html"))
	if !strings.Contains(string(content2), "V2") {
		t.Error("Expected V2 header after fragment change")
	}
}

func TestIntegration_PipelineChange(t *testing.T) {
	pipelineV1 := map[string]string{
		"config.yaml": `name: md
entry_point: index.js
source_suffix: md
target_suffix: html`,
		"index.js": `function transform(ctx, input) {
	return input.replace(/^# (.+)$/gm, '<h1>V1: $1</h1>');
}`,
	}

	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", pipelineV1).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	// Initial build
	_, err := proj.Build(ctx)
	AssertNoError(t, err)

	content1, _ := os.ReadFile(filepath.Join(proj.Config.OutputPath, "index.html"))
	if !strings.Contains(string(content1), "V1:") {
		t.Error("Expected V1 prefix in output")
	}

	// Modify pipeline
	time.Sleep(10 * time.Millisecond)
	pipelineJS := `function transform(ctx, input) {
	return input.replace(/^# (.+)$/gm, '<h1>V2: $1</h1>');
}`
	err = os.WriteFile(
		filepath.Join(proj.Config.PipesPath, "md", "index.js"),
		[]byte(pipelineJS),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to update pipeline: %v", err)
	}

	// Reload pipeline
	err = proj.Pipes.Reload("md")
	if err != nil {
		t.Fatalf("Failed to reload pipeline: %v", err)
	}

	// Rebuild
	_, err = proj.Build(ctx)
	AssertNoError(t, err)

	content2, _ := os.ReadFile(filepath.Join(proj.Config.OutputPath, "index.html"))
	if !strings.Contains(string(content2), "V2:") {
		t.Error("Expected V2 prefix after pipeline change")
	}
}

func TestIntegration_LargeProject(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large project test in short mode")
	}

	proj := NewTestProject(t).
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Build.Concurrency = 4
		})

	// Create 100 files
	for i := 0; i < 100; i++ {
		filename := fmt.Sprintf("page%03d.md", i)
		content := fmt.Sprintf("# Page %d\n\nContent for page %d", i, i)
		proj.WithSource(filename, content)
	}

	p, cleanup := proj.Build()
	defer cleanup()

	ctx := context.Background()

	// Time the build
	start := time.Now()
	stats, err := p.Build(ctx)
	duration := time.Since(start)

	AssertNoError(t, err)

	if stats.BuiltEntries != 100 {
		t.Errorf("Expected 100 built entries, got %d", stats.BuiltEntries)
	}

	t.Logf("Built 100 files in %v (%.2f files/sec)",
		duration, float64(100)/duration.Seconds())

	// Incremental build should be much faster
	start = time.Now()
	stats2, err := p.Build(ctx)
	duration2 := time.Since(start)

	AssertNoError(t, err)

	if stats2.DirtyEntries != 0 {
		t.Error("Expected no dirty entries on incremental build")
	}

	t.Logf("Incremental build (no changes) took %v", duration2)

	if duration2 > duration/2 {
		t.Logf("Warning: Incremental build not significantly faster")
	}
}

func TestIntegration_ErrorHandling(t *testing.T) {
	// Pipeline that throws an error
	badPipeline := map[string]string{
		"config.yaml": `name: bad
entry_point: index.js
source_suffix: md
target_suffix: html`,
		"index.js": `function transform(ctx, input) {
	throw new Error("Pipeline error");
}`,
	}

	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", `<?xml version="1.0" encoding="UTF-8"?>
<Layout tpl="layout.html">
	<Content name="content" src="*.md" pipe="bad" />
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

	// Should complete but with errors
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

func TestIntegration_VariableSubstitution(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", BasicLayoutXML()).
		WithScheme("layout.html", BasicLayoutHTML()).
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.Variables = map[string]any{
				"siteTitle": "My Site",
				"author":    "John Doe",
			}
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	_, err := proj.Build(ctx)
	AssertNoError(t, err)

	// Variables should be available to pipelines
	// (This would require a pipeline that uses variables)
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
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	_, err := proj.Build(ctx)
	AssertNoError(t, err)

	AssertFileExists(t, proj.Config.OutputPath, "keep.html")
	AssertFileExists(t, proj.Config.OutputPath, "remove.html")

	// Remove source file
	os.Remove(filepath.Join(proj.Config.SourcePath, "remove.md"))

	// Build with clean
	_, err = proj.Build(ctx)
	AssertNoError(t, err)

	// Verify stale file is removed
	removePath := filepath.Join(proj.Config.OutputPath, "remove.html")
	if _, err := os.Stat(removePath); !os.IsNotExist(err) {
		t.Error("Expected remove.html to be deleted")
	}

	// Keep file should still exist
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
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	stats1, err := proj.Build(ctx)
	AssertNoError(t, err)

	cacheSize1 := stats1.BytesCached

	// Delete output
	os.RemoveAll(proj.Config.OutputPath)
	os.MkdirAll(proj.Config.OutputPath, 0755)

	// Build again - should restore from CAS
	stats2, err := proj.Build(ctx)
	AssertNoError(t, err)

	if stats2.CacheHits == 0 {
		t.Error("Expected cache hits when rebuilding from CAS")
	}

	// Output should be recreated
	AssertFileExists(t, proj.Config.OutputPath, "index.html")

	// Cache size shouldn't grow (same content)
	if stats2.BytesCached > cacheSize1 {
		t.Logf("Cache grew from %d to %d bytes", cacheSize1, stats2.BytesCached)
	}
}
