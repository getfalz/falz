package project

import (
	"testing"
)

func contentEntry(path, tag string) *BuildEntry {
	return &BuildEntry{
		Type:       ContentType,
		SourceRoot: SourceTypeSource,
		SourcePath: path,
		TagName:    tag,
	}
}

func TestGraph_AddNode_GetNode(t *testing.T) {
	g := NewDependencyGraph()
	node := NewEntryNode(contentEntry("about.md", "body"))

	g.AddNode(node)

	got, ok := g.GetNode(node.ID())
	if !ok || got != node {
		t.Fatalf("expected to get back the added node, got %v (ok=%v)", got, ok)
	}
}

func TestGraph_AddNode_ReplacesExistingByID(t *testing.T) {
	g := NewDependencyGraph()
	entry := contentEntry("about.md", "body")
	node1 := NewEntryNode(entry)
	g.AddNode(node1)

	node2 := NewEntryNode(contentEntry("about.md", "body"))
	g.AddNode(node2)

	got, ok := g.GetNode(node1.ID())
	if !ok || got != node2 {
		t.Fatalf("expected node to be replaced by the second insert, got %v (ok=%v)", got, ok)
	}
	if len(g.AllEntries()) != 1 {
		t.Fatalf("expected exactly 1 entry after replace, got %d", len(g.AllEntries()))
	}
}

func TestGraph_AddNode_NilIsNoop(t *testing.T) {
	g := NewDependencyGraph()
	g.AddNode(nil)
	if len(g.AllNodes()) != 0 {
		t.Fatalf("expected no nodes after adding nil")
	}
}

func TestGraph_AllEntries_OnlyReturnsEntryNodes(t *testing.T) {
	g := NewDependencyGraph()
	entryNode := NewEntryNode(contentEntry("about.md", "body"))
	layoutNode := NewLayoutNode(LayoutKey{LayoutPath: "layout.xml"})

	g.AddNode(entryNode)
	g.AddNode(layoutNode)

	entries := g.AllEntries()
	if len(entries) != 1 || entries[0].SourcePath != "about.md" {
		t.Fatalf("expected only the entry node's BuildEntry, got %+v", entries)
	}
}

func TestGraph_EntriesBySource_IndexesAndUnindexesOnRemove(t *testing.T) {
	g := NewDependencyGraph()
	entry := contentEntry("about.md", "body")
	node := NewEntryNode(entry)
	g.AddNode(node)

	found := g.EntriesBySource(SourceTypeSource, ContentType, "about.md")
	if len(found) != 1 || found[0] != entry {
		t.Fatalf("expected entry to be indexed by source, got %+v", found)
	}

	g.RemoveNode(node)

	found = g.EntriesBySource(SourceTypeSource, ContentType, "about.md")
	if len(found) != 0 {
		t.Fatalf("expected entry to be unindexed after removal, got %+v", found)
	}
}

func TestGraph_EntryByID(t *testing.T) {
	g := NewDependencyGraph()
	entry := contentEntry("about.md", "body")
	node := NewEntryNode(entry)
	g.AddNode(node)

	got, ok := g.EntryByID(entry.ID)
	if !ok || got != entry {
		t.Fatalf("expected to find entry by ID, got %+v (ok=%v)", got, ok)
	}

	if _, ok := g.EntryByID("does-not-exist"); ok {
		t.Fatalf("expected not found for unknown ID")
	}

	layoutNode := NewLayoutNode(LayoutKey{LayoutPath: "layout.xml"})
	g.AddNode(layoutNode)
	if _, ok := g.EntryByID(layoutNode.ID()); ok {
		t.Fatalf("expected EntryByID to reject a non-entry node")
	}
}

func TestGraph_DirtyAndCleanEntries(t *testing.T) {
	g := NewDependencyGraph()
	dirtyEntry := contentEntry("a.md", "body")
	cleanEntry := contentEntry("b.md", "body")

	dirtyNode := NewEntryNode(dirtyEntry)
	cleanNode := NewEntryNode(cleanEntry)
	g.AddNode(dirtyNode)
	g.AddNode(cleanNode)

	if err := g.MarkDirty(dirtyNode.ID()); err != nil {
		t.Fatalf("MarkDirty: %v", err)
	}

	dirty := g.DirtyEntries()
	if len(dirty) != 1 || dirty[0] != dirtyEntry {
		t.Fatalf("expected only dirtyEntry to be dirty, got %+v", dirty)
	}

	clean := g.CleanEntries()
	if len(clean) != 1 || clean[0] != cleanEntry {
		t.Fatalf("expected only cleanEntry to be clean, got %+v", clean)
	}

	if err := g.MarkClean(dirtyNode.ID()); err != nil {
		t.Fatalf("MarkClean: %v", err)
	}
	if len(g.DirtyEntries()) != 0 {
		t.Fatalf("expected no dirty entries after MarkClean")
	}
}

func TestGraph_MarkAllDirty_And_ClearDirty(t *testing.T) {
	g := NewDependencyGraph()
	g.AddNode(NewEntryNode(contentEntry("a.md", "body")))
	g.AddNode(NewEntryNode(contentEntry("b.md", "body")))

	if err := g.MarkAllDirty(); err != nil {
		t.Fatalf("MarkAllDirty: %v", err)
	}
	if len(g.DirtyEntries()) != 2 {
		t.Fatalf("expected all 2 entries dirty, got %d", len(g.DirtyEntries()))
	}

	if err := g.ClearDirty(); err != nil {
		t.Fatalf("ClearDirty: %v", err)
	}
	if len(g.DirtyEntries()) != 0 {
		t.Fatalf("expected no dirty entries after ClearDirty")
	}
}

func TestGraph_UpdateEntry_Errors(t *testing.T) {
	g := NewDependencyGraph()

	if err := g.UpdateEntry("missing", func(e *BuildEntry) error { return nil }); err == nil {
		t.Fatalf("expected error for missing node")
	}

	layoutNode := NewLayoutNode(LayoutKey{LayoutPath: "layout.xml"})
	g.AddNode(layoutNode)
	if err := g.UpdateEntry(layoutNode.ID(), func(e *BuildEntry) error { return nil }); err == nil {
		t.Fatalf("expected error for non-entry node")
	}
}

func TestGraph_DependentsOf_DependenciesOf(t *testing.T) {
	g := NewDependencyGraph()
	entryNode := NewEntryNode(contentEntry("about.md", "body"))
	layoutNode := NewLayoutNode(LayoutKey{LayoutPath: "layout.xml"})

	g.AddNode(entryNode)
	g.AddNode(layoutNode)
	g.AddEdge(entryNode, layoutNode) // entry depends on layout

	deps := g.DependenciesOf(entryNode)
	if len(deps) != 1 || deps[0].ID() != layoutNode.ID() {
		t.Fatalf("expected entry's dependency to be layout, got %+v", deps)
	}

	dependents := g.DependentsOf(layoutNode)
	if len(dependents) != 1 || dependents[0].ID() != entryNode.ID() {
		t.Fatalf("expected layout's dependent to be entry, got %+v", dependents)
	}
}

func TestGraph_RemoveNode_RemovesEdgesBothDirections(t *testing.T) {
	g := NewDependencyGraph()
	entryNode := NewEntryNode(contentEntry("about.md", "body"))
	layoutNode := NewLayoutNode(LayoutKey{LayoutPath: "layout.xml"})

	g.AddNode(entryNode)
	g.AddNode(layoutNode)
	g.AddEdge(entryNode, layoutNode)

	g.RemoveNode(layoutNode)

	if deps := g.DependenciesOf(entryNode); len(deps) != 0 {
		t.Fatalf("expected no dependencies left after removing layout node, got %+v", deps)
	}
	if _, ok := g.GetNode(layoutNode.ID()); ok {
		t.Fatalf("expected layout node to be gone")
	}
}

func TestGraph_RemoveEdgesFrom_And_RemoveEdgesTo(t *testing.T) {
	g := NewDependencyGraph()
	a := NewEntryNode(contentEntry("a.md", "body"))
	b := NewLayoutNode(LayoutKey{LayoutPath: "b.xml"})
	c := NewPipelineNode("md")

	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)
	g.AddEdge(a, b)
	g.AddEdge(a, c)

	g.RemoveEdgesFrom(a)
	if deps := g.DependenciesOf(a); len(deps) != 0 {
		t.Fatalf("expected no outgoing edges from a after RemoveEdgesFrom, got %+v", deps)
	}
	if dependents := g.DependentsOf(b); len(dependents) != 0 {
		t.Fatalf("expected reverse index cleared too, got %+v", dependents)
	}

	g.AddEdge(a, b)
	g.RemoveEdgesTo(b)
	if dependents := g.DependentsOf(b); len(dependents) != 0 {
		t.Fatalf("expected no incoming edges to b after RemoveEdgesTo, got %+v", dependents)
	}
	if deps := g.DependenciesOf(a); len(deps) != 0 {
		t.Fatalf("expected forward index cleared too, got %+v", deps)
	}
}

// TestGraph_AffectedBy_TransitiveBFS builds A -> B -> C (A depends on B,
// B depends on C) and checks that a change to C is reported as affecting
// both B and A.
func TestGraph_AffectedBy_TransitiveBFS(t *testing.T) {
	g := NewDependencyGraph()
	a := NewEntryNode(contentEntry("a.md", "body"))
	b := NewLayoutNode(LayoutKey{LayoutPath: "b.xml"})
	c := NewFragmentNode("c.frag")

	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)
	g.AddEdge(a, b)
	g.AddEdge(b, c)

	affected := g.AffectedBy(c)

	ids := map[string]bool{}
	for _, n := range affected {
		ids[n.ID()] = true
	}
	if !ids[a.ID()] || !ids[b.ID()] {
		t.Fatalf("expected both a and b to be affected by a change to c, got %+v", affected)
	}
}

func TestGraph_Clear_ResetsEverything(t *testing.T) {
	g := NewDependencyGraph()
	entryNode := NewEntryNode(contentEntry("about.md", "body"))
	layoutNode := NewLayoutNode(LayoutKey{LayoutPath: "layout.xml"})
	g.AddNode(entryNode)
	g.AddNode(layoutNode)
	g.AddEdge(entryNode, layoutNode)

	g.Clear()

	if len(g.AllNodes()) != 0 {
		t.Fatalf("expected no nodes after Clear")
	}
	if len(g.AllEntries()) != 0 {
		t.Fatalf("expected no entries after Clear")
	}
	if deps := g.DependenciesOf(entryNode); len(deps) != 0 {
		t.Fatalf("expected no edges after Clear, got %+v", deps)
	}
}

func TestNewEntryNode_PanicsOnNilEntry(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic for nil entry")
		}
	}()
	NewEntryNode(nil)
}

func TestBuildEntry_CanonicalID(t *testing.T) {
	tests := []struct {
		name  string
		entry *BuildEntry
	}{
		{
			name:  "content",
			entry: &BuildEntry{Type: ContentType, SourcePath: "a.md", TagName: "body"},
		},
		{
			name:  "source asset",
			entry: &BuildEntry{Type: AssetType, SourceRoot: SourceTypeSource, SourcePath: "a.css", TagName: "style"},
		},
		{
			name:  "scheme asset",
			entry: &BuildEntry{Type: AssetType, SourceRoot: SourceTypeScheme, SourcePath: "a.css", TagName: "style"},
		},
	}

	seen := map[string]string{}
	for _, tt := range tests {
		id := tt.entry.CanonicalID()
		if id == "" {
			t.Errorf("%s: expected non-empty canonical ID", tt.name)
		}
		if prev, ok := seen[id]; ok {
			t.Errorf("%s: canonical ID %q collides with %q", tt.name, id, prev)
		}
		seen[id] = tt.name
	}
}

func TestBuildEntry_CanonicalID_PanicsOnUnsupportedCombination(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic for unsupported entry identity")
		}
	}()
	e := &BuildEntry{Type: AssetType, SourceRoot: SourceTypeOutput, SourcePath: "a.css"}
	_ = e.CanonicalID()
}
