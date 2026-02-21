package project

import (
	"context"
	"testing"
)

// =============================================================================
// Real-World Scenario Tests for SourceIndex
// =============================================================================

// TestSourceIndex_BasicContentScan tests scanning simple content files.
func TestSourceIndex_BasicContentScan(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Home").
		WithSource("about.md", "# About").
		WithScheme("layout.xml", `
			<layout>
				<content path="*.md" to="{FILENAME}.html" pipe="markdown" name="post"/>
			</layout>
		`).
		WithScheme("layout.html", "<html>{{content}}</html>").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	err := scanner.Scan(context.Background(), graph)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should have 2 entries
	entries := graph.AllEntries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Check entries exist
	entry1, ok := graph.EntryByID("index.md")
	if !ok {
		t.Error("index.md not found")
	}
	entry2, ok := graph.EntryByID("about.md")
	if !ok {
		t.Error("about.md not found")
	}

	// Verify layout keys are set
	if entry1.LayoutKey.LayoutPath == "" {
		t.Error("entry1 has no layout key")
	}
	if entry2.LayoutKey.LayoutPath == "" {
		t.Error("entry2 has no layout key")
	}
}

// TestSourceIndex_FragmentOverride tests fragment override behavior.
// This is a critical scenario that caused bugs.
func TestSourceIndex_FragmentOverride(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		// Root content
		WithSource("index.md", "# Home").
		// Blog content
		WithSource("blog/post.md", "# Post").
		// Root layout and fragments
		WithScheme("layout.xml", `
			<layout>
				<fragment name="header" source="header.html"/>
				<content path="*.md" to="{FILENAME}.html" pipe="markdown" name="post"/>
			</layout>
		`).
		WithScheme("layout.html", "<html>{{header}}{{content}}</html>").
		WithScheme("header.html", "<header>Default Header</header>").
		// Blog fragment override
		WithScheme("blog/header.html", "<header>Blog Header</header>").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	err := scanner.Scan(context.Background(), graph)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// CRITICAL: Should have TWO different layout nodes
	// Same layout.xml file, but different fragment contexts
	// stats := graph.Stats()
	// if stats.LayoutNodes != 2 {
	// 	t.Errorf("expected 2 layout nodes (root + blog context), got %d", stats.LayoutNodes)
	// }

	// Check root entry uses root context
	rootEntry, _ := graph.EntryByID("index.md")
	if rootEntry.LayoutKey.CtxPath != "." {
		t.Errorf("root entry has wrong fragment context: %s", rootEntry.LayoutKey.CtxPath)
	}

	// Check blog entry uses blog context
	blogEntry, _ := graph.EntryByID("blog/post.md")
	if blogEntry.LayoutKey.CtxPath != "blog" {
		t.Errorf("blog entry has wrong fragment context: %s", blogEntry.LayoutKey.CtxPath)
	}

	// Verify fragment nodes exist
	rootLayoutNode := NewLayoutNode(rootEntry.LayoutKey)
	rootDeps := graph.DependenciesOf(rootLayoutNode)
	hasDefaultFragment := false
	for _, dep := range rootDeps {
		if dep.ID() == "fragment:header.html" {
			hasDefaultFragment = true
		}
	}
	if !hasDefaultFragment {
		t.Error("root layout should depend on header.html")
	}

	blogLayoutNode := NewLayoutNode(blogEntry.LayoutKey)
	blogDeps := graph.DependenciesOf(blogLayoutNode)
	hasOverrideFragment := false
	for _, dep := range blogDeps {
		if dep.ID() == "fragment:blog/header.html" {
			hasOverrideFragment = true
		}
	}
	if !hasOverrideFragment {
		t.Error("blog layout should depend on blog/header.html")
	}
}

// TestSourceIndex_LayoutInheritance tests layout inheritance from parent directories.
func TestSourceIndex_LayoutInheritance(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		// Content in subdirectory (no layout)
		WithSource("blog/tech/article.md", "# Article").
		// Layout in root only
		WithScheme("layout.xml", `
			<layout>
				<content path="**/*.md" to="{FILENAME}.html" pipe="markdown" name="post"/>
			</layout>
		`).
		WithScheme("layout.html", "<html>{{content}}</html>").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	err := scanner.Scan(context.Background(), graph)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Entry should inherit layout from root
	entry, ok := graph.EntryByID("blog/tech/article.md")
	if !ok {
		t.Fatal("article.md not found")
	}

	// Layout should be from root, but fragment context is blog/tech
	if entry.LayoutKey.LayoutPath != "layout.xml" {
		t.Errorf("expected layout.xml, got %s", entry.LayoutKey.LayoutPath)
	}
	if entry.LayoutKey.CtxPath != "blog/tech" {
		t.Errorf("expected fragment context blog/tech, got %s", entry.LayoutKey.CtxPath)
	}
}

// TestSourceIndex_MultipleDirectoriesWithSameLayout tests the bug where
// multiple directories using the same layout should create different layout nodes.
func TestSourceIndex_MultipleDirectoriesWithSameLayout(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("blog/post1.md", "# Post 1").
		WithSource("docs/guide.md", "# Guide").
		WithScheme("layout.xml", `
			<layout>
				<fragment name="header" source="header.html"/>
				<content path="*.md" to="{FILENAME}.html" pipe="markdown" name="post"/>
			</layout>
		`).
		WithScheme("layout.html", "<html>{{header}}{{content}}</html>").
		WithScheme("header.html", "Default").
		WithScheme("blog/header.html", "Blog Header").
		WithScheme("docs/header.html", "Docs Header").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	err := scanner.Scan(context.Background(), graph)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should have 2 layout nodes (blog context + docs context)
	// stats := graph.Stats()
	// if stats.LayoutNodes != 2 {
	// 	t.Errorf("expected 2 layout nodes, got %d", stats.LayoutNodes)
	// }

	// Each entry should have correct fragment context
	blogEntry, _ := graph.EntryByID("blog/post1.md")
	if blogEntry.LayoutKey.CtxPath != "blog" {
		t.Errorf("blog entry wrong context: %s", blogEntry.LayoutKey.CtxPath)
	}

	docsEntry, _ := graph.EntryByID("docs/guide.md")
	if docsEntry.LayoutKey.CtxPath != "docs" {
		t.Errorf("docs entry wrong context: %s", docsEntry.LayoutKey.CtxPath)
	}

	// Verify blog layout depends on blog/header.html
	blogLayout := NewLayoutNode(blogEntry.LayoutKey)
	blogDeps := graph.DependenciesOf(blogLayout)
	hasBlogHeader := false
	for _, dep := range blogDeps {
		if dep.ID() == "fragment:blog/header.html" {
			hasBlogHeader = true
		}
	}
	if !hasBlogHeader {
		t.Error("blog layout should depend on blog/header.html")
	}

	// Verify docs layout depends on docs/header.html
	docsLayout := NewLayoutNode(docsEntry.LayoutKey)
	docsDeps := graph.DependenciesOf(docsLayout)
	hasDocsHeader := false
	for _, dep := range docsDeps {
		if dep.ID() == "fragment:docs/header.html" {
			hasDocsHeader = true
		}
	}
	if !hasDocsHeader {
		t.Error("docs layout should depend on docs/header.html")
	}
}

// TestSourceIndex_PartialScan tests partial scanning with layout boundaries.
func TestSourceIndex_PartialScan(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		// Parent directory content
		WithSource("blog/post.md", "# Post").
		// Subdirectory with own layout
		WithSource("blog/tech/article.md", "# Article").
		// Parent layout
		WithScheme("blog/layout.xml", `
			<layout>
				<content path="*.md" to="{FILENAME}.html" pipe="markdown" name="post"/>
			</layout>
		`).
		WithScheme("blog/layout.html", "<html>{{content}}</html>").
		// Subdirectory layout
		WithScheme("blog/tech/layout.xml", `
			<layout>
				<content path="*.md" to="{FILENAME}.html" pipe="markdown" name="article"/>
			</layout>
		`).
		WithScheme("blog/tech/layout.html", "<html><div>{{content}}</div></html>").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	// Partial scan of blog/ should STOP at blog/tech/ (has own layout)
	err := scanner.PartialScan(context.Background(), graph, "blog")
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	// Should only have blog/post.md, NOT blog/tech/article.md
	entries := graph.AllEntries()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (blog/post.md), got %d", len(entries))
	}

	_, hasBlogPost := graph.EntryByID("blog/post.md")
	if !hasBlogPost {
		t.Error("blog/post.md should exist")
	}

	_, hasTechArticle := graph.EntryByID("blog/tech/article.md")
	if hasTechArticle {
		t.Error("blog/tech/article.md should NOT exist (stopped at layout boundary)")
	}
}

// TestSourceIndex_PipelineDependencies tests pipeline node creation.
func TestSourceIndex_PipelineDependencies(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("post.md", "# Post").
		WithSource("page.html", "<p>Page</p>").
		WithScheme("layout.xml", `
			<layout>
				<content path="*.md" to="{FILENAME}.html" pipe="markdown" name="post"/>
				<content path="*.html" to="{FILENAME}.html" pipe="copy" name="page"/>
			</layout>
		`).
		WithScheme("layout.html", "<html>{{content}}</html>").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	err := scanner.Scan(context.Background(), graph)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should have 2 pipeline nodes
	// stats := graph.Stats()
	// if stats.PipelineNodes != 2 {
	// 	t.Errorf("expected 2 pipeline nodes (markdown + copy), got %d", stats.PipelineNodes)
	// }

	// Verify entries depend on correct pipelines
	mdEntry, _ := graph.EntryByID("post.md")
	if mdEntry.Pipeline != "markdown" {
		t.Errorf("post.md should use markdown pipeline, got %s", mdEntry.Pipeline)
	}

	htmlEntry, _ := graph.EntryByID("page.html")
	if htmlEntry.Pipeline != "copy" {
		t.Errorf("page.html should use copy pipeline, got %s", htmlEntry.Pipeline)
	}

	// Verify graph edges
	mdNode := NewEntryNode(mdEntry)
	mdDeps := graph.DependenciesOf(mdNode)
	hasMdPipeline := false
	for _, dep := range mdDeps {
		if dep.Kind() == NodeKindPipeline && dep.ID() == "pipeline:markdown" {
			hasMdPipeline = true
		}
	}
	if !hasMdPipeline {
		t.Error("markdown entry should depend on markdown pipeline")
	}
}

// TestSourceIndex_ComplexProject tests a realistic complex project structure.
func TestSourceIndex_ComplexProject(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		// Root
		WithSource("index.md", "# Home").
		// Blog section
		WithSource("blog/post1.md", "# Post 1").
		WithSource("blog/post2.md", "# Post 2").
		// Docs section with subsections
		WithSource("docs/guide.md", "# Guide").
		WithSource("docs/api/reference.md", "# API").
		// Root layout
		WithScheme("layout.xml", `
			<layout>
				<fragment name="header" source="header.html"/>
				<fragment name="footer" source="footer.html"/>
				<content path="**/*.md" to="{FILENAME}.html" pipe="markdown" name="content"/>
			</layout>
		`).
		WithScheme("layout.html", "{{header}}{{content}}{{footer}}").
		WithScheme("header.html", "Default Header").
		WithScheme("footer.html", "Default Footer").
		// Blog overrides
		WithScheme("blog/header.html", "Blog Header").
		// Docs overrides
		WithScheme("docs/header.html", "Docs Header").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	err := scanner.Scan(context.Background(), graph)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should have entries for all content
	entries := graph.AllEntries()
	expectedEntries := 5 // index, 2 blog posts, 1 guide, 1 api reference
	if len(entries) != expectedEntries {
		t.Errorf("expected %d entries, got %d", expectedEntries, len(entries))
	}

	// Should have layout nodes for each context
	// stats := graph.Stats()
	// expectedLayouts := 4 // root, blog, docs, docs/api
	// if stats.LayoutNodes != expectedLayouts {
	// 	t.Errorf("expected %d layout nodes, got %d", expectedLayouts, stats.LayoutNodes)
	// }

	// Verify fragment override contexts
	blogEntry, _ := graph.EntryByID("blog/post1.md")
	if blogEntry.LayoutKey.CtxPath != "blog" {
		t.Errorf("blog entry wrong context: %s", blogEntry.LayoutKey.CtxPath)
	}

	docsEntry, _ := graph.EntryByID("docs/guide.md")
	if docsEntry.LayoutKey.CtxPath != "docs" {
		t.Errorf("docs entry wrong context: %s", docsEntry.LayoutKey.CtxPath)
	}

	apiEntry, _ := graph.EntryByID("docs/api/reference.md")
	if apiEntry.LayoutKey.CtxPath != "docs/api" {
		t.Errorf("api entry wrong context: %s", apiEntry.LayoutKey.CtxPath)
	}
}

// TestSourceIndex_FragmentChangeAffectsCorrectEntries tests that fragment
// changes only affect entries in the correct context (not all entries).
func TestSourceIndex_FragmentChangeAffectsCorrectEntries(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Home").
		WithSource("blog/post.md", "# Post").
		WithScheme("layout.xml", `
			<layout>
				<fragment name="header" source="header.html"/>
				<content path="**/*.md" to="{FILENAME}.html" pipe="markdown" name="content"/>
			</layout>
		`).
		WithScheme("layout.html", "{{header}}{{content}}").
		WithScheme("header.html", "Default").
		WithScheme("blog/header.html", "Blog Header").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	err := scanner.Scan(context.Background(), graph)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Simulate blog/header.html change
	blogHeaderNode := NewFragmentNode("blog/header.html")
	affected := graph.AffectedBy(blogHeaderNode)

	// Should affect blog layout and blog/post.md
	// Should NOT affect root layout or index.md
	affectedEntryCount := 0
	affectedIndex := false
	affectedBlogPost := false

	for _, node := range affected {
		if node.Kind() == NodeKindEntry {
			affectedEntryCount++
			if node.ID() == "index.md" {
				affectedIndex = true
			}
			if node.ID() == "blog/post.md" {
				affectedBlogPost = true
			}
		}
	}

	if affectedIndex {
		t.Error("index.md should NOT be affected by blog/header.html change")
	}

	if !affectedBlogPost {
		t.Error("blog/post.md SHOULD be affected by blog/header.html change")
	}

	if affectedEntryCount != 1 {
		t.Errorf("expected 1 affected entry, got %d", affectedEntryCount)
	}
}

// TestSourceIndex_ConcurrentScanning tests thread safety.
func TestSourceIndex_ConcurrentScanning(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("file1.md", "# File 1").
		WithSource("file2.md", "# File 2").
		WithSource("file3.md", "# File 3").
		WithScheme("layout.xml", `
			<layout>
				<content path="*.md" to="{FILENAME}.html" pipe="markdown" name="post"/>
			</layout>
		`).
		WithScheme("layout.html", "<html>{{content}}</html>").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	// Scan multiple times concurrently
	done := make(chan bool, 3)
	for i := 0; i < 3; i++ {
		go func() {
			err := scanner.Scan(context.Background(), graph)
			if err != nil {
				t.Errorf("Concurrent scan failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	// Should still have correct number of entries (no duplicates)
	entries := graph.AllEntries()
	if len(entries) != 3 {
		t.Errorf("expected 3 entries after concurrent scans, got %d", len(entries))
	}
}

// TestSourceIndex_ContextCancellation tests context cancellation.
func TestSourceIndex_ContextCancellation(t *testing.T) {
	proj, cleanup := NewTestProject(t).
		WithSource("file1.md", "# File 1").
		WithSource("file2.md", "# File 2").
		WithScheme("layout.xml", `
			<layout>
				<content path="*.md" to="{FILENAME}.html" pipe="markdown" name="post"/>
			</layout>
		`).
		WithScheme("layout.html", "<html>{{content}}</html>").
		Build()
	defer cleanup()

	graph := NewDependencyGraph()
	scanner := NewSourceIndex(proj)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := scanner.Scan(ctx, graph)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
