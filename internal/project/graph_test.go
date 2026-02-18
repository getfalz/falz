package project

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestFragmentChangeAffectsMultipleEntries simulates a real fragment change
// and verifies all affected entries are found.
func TestGraph_FragmentChangeAffectsMultipleEntries(t *testing.T) {
	g := NewDependencyGraph()

	// Setup: Blog with shared header fragment
	header := NewFragmentNode("header.html")
	layout := NewLayoutNode(LayoutKey{BaseDir: "blog", RelLayoutPath: "layout.xml"})

	post1 := NewEntryNode(&BuildEntry{ID: "blog/post1.md", Dirty: false})
	post2 := NewEntryNode(&BuildEntry{ID: "blog/post2.md", Dirty: false})
	post3 := NewEntryNode(&BuildEntry{ID: "blog/post3.md", Dirty: false})

	// Build graph
	g.AddNode(header)
	g.AddNode(layout)
	g.AddNode(post1)
	g.AddNode(post2)
	g.AddNode(post3)

	// Edges: entries -> layout -> header
	g.AddEdge(post1, layout)
	g.AddEdge(post2, layout)
	g.AddEdge(post3, layout)
	g.AddEdge(layout, header)

	// Simulate: header.html changes
	affected := g.AffectedBy(header)

	// Should affect layout + all 3 posts
	if len(affected) != 4 {
		t.Fatalf("expected 4 affected nodes, got %d", len(affected))
	}

	// Mark all affected entries dirty
	for _, node := range affected {
		if node.Kind() == NodeKindEntry {
			g.MarkDirty(node.ID())
		}
	}

	// Verify all posts are now dirty
	dirty := g.DirtyEntries()
	if len(dirty) != 3 {
		t.Errorf("expected 3 dirty entries, got %d", len(dirty))
	}
}

// TestPipelineChangeAffectsOnlyMatchingEntries simulates a pipeline change
// and verifies only entries using that pipeline are affected.
func TestGraph_PipelineChangeAffectsOnlyMatchingEntries(t *testing.T) {
	g := NewDependencyGraph()

	// Setup: Mix of markdown and copy pipelines
	markdown := NewPipelineNode("markdown")
	copy := NewPipelineNode("copy")

	mdPost := NewEntryNode(&BuildEntry{ID: "post.md", Pipeline: "markdown", Dirty: false})
	mdGuide := NewEntryNode(&BuildEntry{ID: "guide.md", Pipeline: "markdown", Dirty: false})
	cssFile := NewEntryNode(&BuildEntry{ID: "style.css", Pipeline: "copy", Dirty: false})

	g.AddNode(markdown)
	g.AddNode(copy)
	g.AddNode(mdPost)
	g.AddNode(mdGuide)
	g.AddNode(cssFile)

	g.AddEdge(mdPost, markdown)
	g.AddEdge(mdGuide, markdown)
	g.AddEdge(cssFile, copy)

	// Simulate: markdown pipeline changes
	affected := g.AffectedBy(markdown)

	// Should only affect the 2 markdown entries
	if len(affected) != 2 {
		t.Fatalf("expected 2 affected nodes, got %d", len(affected))
	}

	// Verify CSS file is NOT affected
	for _, node := range affected {
		if node.ID() == "style.css" {
			t.Error("CSS file should not be affected by markdown pipeline change")
		}
	}

	// Mark affected entries dirty
	for _, node := range affected {
		if node.Kind() == NodeKindEntry {
			g.MarkDirty(node.ID())
		}
	}

	dirty := g.DirtyEntries()
	if len(dirty) != 2 {
		t.Errorf("expected 2 dirty entries, got %d", len(dirty))
	}
}

// TestIncrementalBuildAfterFragmentChange simulates a complete incremental build cycle.
func TestGraph_IncrementalBuildAfterFragmentChange(t *testing.T) {
	g := NewDependencyGraph()

	// Initial state: 5 blog posts, all clean
	layout := NewLayoutNode(LayoutKey{BaseDir: "blog", RelLayoutPath: "layout.xml"})
	header := NewFragmentNode("header.html")

	g.AddNode(layout)
	g.AddNode(header)
	g.AddEdge(layout, header)

	var entries []*BuildEntry
	for i := 1; i <= 5; i++ {
		entry := &BuildEntry{
			ID:    fmt.Sprintf("blog/post%d.md", i),
			Dirty: false,
			Hash:  fmt.Sprintf("hash%d", i),
		}
		entries = append(entries, entry)

		node := NewEntryNode(entry)
		g.AddNode(node)
		g.AddEdge(node, layout)
	}

	// Verify all clean initially
	if len(g.DirtyEntries()) != 0 {
		t.Fatal("expected all entries clean initially")
	}

	// Simulate: header.html changes
	affected := g.AffectedBy(header)
	for _, node := range affected {
		if node.Kind() == NodeKindEntry {
			g.UpdateEntry(node.ID(), func(e *BuildEntry) error {
				e.Dirty = true
				e.Hash = "newhash" // Simulating recompute
				return nil
			})
		}
	}

	// All 5 should be dirty now
	dirty := g.DirtyEntries()
	if len(dirty) != 5 {
		t.Errorf("expected 5 dirty entries, got %d", len(dirty))
	}

	// Simulate: build completes, mark clean
	for _, entry := range dirty {
		g.MarkClean(entry.ID)
	}

	// All should be clean again
	if len(g.DirtyEntries()) != 0 {
		t.Error("expected all entries clean after build")
	}
}

// TestMultipleLayoutsWithSharedFragments tests a complex scenario where
// multiple layouts share some fragments.
func TestGraph_MultipleLayoutsWithSharedFragments(t *testing.T) {
	g := NewDependencyGraph()

	// Shared fragments
	header := NewFragmentNode("header.html")
	footer := NewFragmentNode("footer.html")

	// Different layouts
	blogLayout := NewLayoutNode(LayoutKey{BaseDir: "blog", RelLayoutPath: "layout.xml"})
	docsLayout := NewLayoutNode(LayoutKey{BaseDir: "docs", RelLayoutPath: "layout.xml"})

	// Entries
	blogPost := NewEntryNode(&BuildEntry{ID: "blog/post.md", Dirty: false})
	docsGuide := NewEntryNode(&BuildEntry{ID: "docs/guide.md", Dirty: false})

	// Build graph
	g.AddNode(header)
	g.AddNode(footer)
	g.AddNode(blogLayout)
	g.AddNode(docsLayout)
	g.AddNode(blogPost)
	g.AddNode(docsGuide)

	// Both layouts use header and footer
	g.AddEdge(blogLayout, header)
	g.AddEdge(blogLayout, footer)
	g.AddEdge(docsLayout, header)
	g.AddEdge(docsLayout, footer)

	// Entries use their layouts
	g.AddEdge(blogPost, blogLayout)
	g.AddEdge(docsGuide, docsLayout)

	// Simulate: shared header changes
	affected := g.AffectedBy(header)

	// Should affect: both layouts + both entries
	if len(affected) != 4 {
		t.Fatalf("expected 4 affected nodes (2 layouts + 2 entries), got %d", len(affected))
	}

	// Count affected entries
	entryCount := 0
	for _, node := range affected {
		if node.Kind() == NodeKindEntry {
			entryCount++
		}
	}

	if entryCount != 2 {
		t.Errorf("expected 2 affected entries, got %d", entryCount)
	}
}

// TestRemoveNodeCleansUpEdges verifies that removing a node cleans up all related edges.
func TestGraph_RemoveNodeCleansUpEdges(t *testing.T) {
	g := NewDependencyGraph()

	// Setup
	pipeline := NewPipelineNode("markdown")
	entry1 := NewEntryNode(&BuildEntry{ID: "post1.md"})
	entry2 := NewEntryNode(&BuildEntry{ID: "post2.md"})

	g.AddNode(pipeline)
	g.AddNode(entry1)
	g.AddNode(entry2)

	g.AddEdge(entry1, pipeline)
	g.AddEdge(entry2, pipeline)

	// Verify pipeline has 2 dependents
	deps := g.DependentsOf(pipeline)
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependents, got %d", len(deps))
	}

	// Remove pipeline node
	g.RemoveNode(pipeline)

	// Verify pipeline is gone
	if _, ok := g.GetNode("pipeline:markdown"); ok {
		t.Error("pipeline should be removed")
	}

	// Verify entries no longer have dependency on pipeline
	entry1Deps := g.DependenciesOf(entry1)
	if len(entry1Deps) != 0 {
		t.Errorf("entry1 should have no dependencies after pipeline removed, got %d", len(entry1Deps))
	}
}

// TestConcurrentBuildsWithSharedLayout tests concurrent builds of entries
// using the same layout.
func TestGraph_ConcurrentBuildsWithSharedLayout(t *testing.T) {
	g := NewDependencyGraph()

	// One layout, many entries
	layout := NewLayoutNode(LayoutKey{BaseDir: "blog", RelLayoutPath: "layout.xml"})
	g.AddNode(layout)

	// Add 100 entries
	for i := 0; i < 100; i++ {
		entry := &BuildEntry{
			ID:    fmt.Sprintf("post%d.md", i),
			Dirty: true,
		}
		node := NewEntryNode(entry)
		g.AddNode(node)
		g.AddEdge(node, layout)
	}

	// Simulate: 10 concurrent "builds" updating entries
	var wg sync.WaitGroup
	wg.Add(10)

	for worker := 0; worker < 10; worker++ {
		go func(w int) {
			defer wg.Done()

			// Each worker processes 10 entries
			for i := w * 10; i < (w+1)*10; i++ {
				entryID := fmt.Sprintf("post%d.md", i)

				// Simulate build: update entry
				g.UpdateEntry(entryID, func(e *BuildEntry) error {
					e.Hash = fmt.Sprintf("built-by-worker-%d", w)
					time.Sleep(time.Microsecond) // Simulate work
					e.Dirty = false
					return nil
				})
			}
		}(worker)
	}

	wg.Wait()

	// All entries should be clean
	dirty := g.DirtyEntries()
	if len(dirty) != 0 {
		t.Errorf("expected all entries clean, got %d dirty", len(dirty))
	}

	// All entries should have hashes
	allEntries := g.AllEntries()
	for _, entry := range allEntries {
		if entry.Hash == "" {
			t.Errorf("entry %s should have hash", entry.ID)
		}
	}
}

// TestComplexDependencyChain tests a deep dependency chain.
func TestGraph_ComplexDependencyChain(t *testing.T) {
	g := NewDependencyGraph()

	// Chain: entry -> layout -> template -> fragment
	fragment := NewFragmentNode("base.html")
	template := NewTemplateNode("template.html")
	layout := NewLayoutNode(LayoutKey{BaseDir: ".", RelLayoutPath: "layout.xml"})
	entry := NewEntryNode(&BuildEntry{ID: "page.md", Dirty: false})

	g.AddNode(fragment)
	g.AddNode(template)
	g.AddNode(layout)
	g.AddNode(entry)

	g.AddEdge(template, fragment)
	g.AddEdge(layout, template)
	g.AddEdge(entry, layout)

	// Change at bottom of chain (fragment)
	affected := g.AffectedBy(fragment)

	// Should propagate through entire chain
	if len(affected) != 3 {
		t.Fatalf("expected 3 affected nodes (template, layout, entry), got %d", len(affected))
	}

	// Verify entry is affected
	entryAffected := false
	for _, node := range affected {
		if node.Kind() == NodeKindEntry && node.ID() == "page.md" {
			entryAffected = true
		}
	}

	if !entryAffected {
		t.Error("entry should be affected by fragment change")
	}
}

// TestBulkOperations tests marking all entries dirty and clean.
func TestGraph_BulkOperations(t *testing.T) {
	g := NewDependencyGraph()

	// Add 1000 entries
	for i := 0; i < 1000; i++ {
		entry := &BuildEntry{
			ID:    fmt.Sprintf("entry%d.md", i),
			Dirty: false,
		}
		g.AddNode(NewEntryNode(entry))
	}

	// Mark all dirty
	if err := g.MarkAllDirty(); err != nil {
		t.Fatalf("MarkAllDirty failed: %v", err)
	}

	// Verify all dirty
	dirty := g.DirtyEntries()
	if len(dirty) != 1000 {
		t.Errorf("expected 1000 dirty entries, got %d", len(dirty))
	}

	// Clear all
	if err := g.ClearDirty(); err != nil {
		t.Fatalf("ClearDirty failed: %v", err)
	}

	// Verify all clean
	dirty = g.DirtyEntries()
	if len(dirty) != 0 {
		t.Errorf("expected 0 dirty entries, got %d", len(dirty))
	}
}

// TestRealisticProjectStructure simulates a real project with blog and docs sections.
func TestGraph_RealisticProjectStructure(t *testing.T) {
	g := NewDependencyGraph()

	// Shared fragments
	header := NewFragmentNode("header.html")
	footer := NewFragmentNode("footer.html")
	g.AddNode(header)
	g.AddNode(footer)

	// Pipelines
	markdown := NewPipelineNode("markdown")
	sass := NewPipelineNode("sass")
	g.AddNode(markdown)
	g.AddNode(sass)

	// Blog section
	blogLayout := NewLayoutNode(LayoutKey{BaseDir: "blog", RelLayoutPath: "layout.xml"})
	g.AddNode(blogLayout)
	g.AddEdge(blogLayout, header)
	g.AddEdge(blogLayout, footer)

	for i := 1; i <= 10; i++ {
		entry := &BuildEntry{ID: fmt.Sprintf("blog/post%d.md", i), Dirty: false}
		node := NewEntryNode(entry)
		g.AddNode(node)
		g.AddEdge(node, blogLayout)
		g.AddEdge(node, markdown)
	}

	// Docs section
	docsLayout := NewLayoutNode(LayoutKey{BaseDir: "docs", RelLayoutPath: "layout.xml"})
	g.AddNode(docsLayout)
	g.AddEdge(docsLayout, header)
	g.AddEdge(docsLayout, footer)

	for i := 1; i <= 5; i++ {
		entry := &BuildEntry{ID: fmt.Sprintf("docs/guide%d.md", i), Dirty: false}
		node := NewEntryNode(entry)
		g.AddNode(node)
		g.AddEdge(node, docsLayout)
		g.AddEdge(node, markdown)
	}

	// Styles
	styleEntry := &BuildEntry{ID: "style.scss", Dirty: false}
	styleNode := NewEntryNode(styleEntry)
	g.AddNode(styleNode)
	g.AddEdge(styleNode, sass)

	// Test: Header changes -> affects all content (blog + docs)
	affected := g.AffectedBy(header)

	entryCount := 0
	for _, node := range affected {
		if node.Kind() == NodeKindEntry {
			entryCount++
		}
	}

	// Should affect 10 blog + 5 docs = 15 entries (not style)
	if entryCount != 15 {
		t.Errorf("expected 15 affected entries, got %d", entryCount)
	}

	// Test: Markdown pipeline changes -> affects only markdown entries
	affected = g.AffectedBy(markdown)

	entryCount = 0
	for _, node := range affected {
		if node.Kind() == NodeKindEntry {
			entryCount++
		}
	}

	// Should affect all 15 markdown entries
	if entryCount != 15 {
		t.Errorf("expected 15 affected entries from markdown, got %d", entryCount)
	}

	// Test: SASS pipeline changes -> affects only style entry
	affected = g.AffectedBy(sass)

	entryCount = 0
	for _, node := range affected {
		if node.Kind() == NodeKindEntry {
			entryCount++
		}
	}

	if entryCount != 1 {
		t.Errorf("expected 1 affected entry from sass, got %d", entryCount)
	}
}
