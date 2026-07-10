package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"craeft/internal/pipeline"
	"craeft/internal/utils"
)

func newTestLayoutRoot(t *testing.T) *utils.RootFS {
	t.Helper()
	root, err := utils.NewRootFs(t.TempDir())
	if err != nil {
		t.Fatalf("new root fs: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func newTestPipesRegistry(t *testing.T) *pipeline.Registry {
	t.Helper()
	reg, err := pipeline.NewRegistry(&pipeline.RegistryConfig{PipesPath: t.TempDir()})
	if err != nil {
		t.Fatalf("new pipes registry: %v", err)
	}
	return reg
}

func writeLayoutFile(t *testing.T, root *utils.RootFS, relPath, content string) {
	t.Helper()
	full := filepath.Join(root.Path(), relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func TestLayoutCache_Parse_CachesParsedLayout(t *testing.T) {
	root := newTestLayoutRoot(t)
	writeLayoutFile(t, root, "layout.xml", `<Layout tpl="x"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`)

	lc := NewLayoutCache(root, newTestPipesRegistry(t), "layout.xml")
	key := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	lyt1, err := lc.Parse(key)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	lyt2, err := lc.Parse(key)
	if err != nil {
		t.Fatalf("Parse (2nd): %v", err)
	}
	if lyt1 != lyt2 {
		t.Fatalf("expected Parse to return the same cached *Layout instance")
	}
}

func TestLayoutCache_Parse_MissingFile_ReturnsError(t *testing.T) {
	root := newTestLayoutRoot(t)
	lc := NewLayoutCache(root, newTestPipesRegistry(t), "layout.xml")

	if _, err := lc.Parse(LayoutKey{LayoutPath: "does-not-exist.xml"}); err == nil {
		t.Fatalf("expected error for a missing layout file")
	}
}

func TestLayoutCache_Get_RendersFragmentsAndCompilesContentTemplate(t *testing.T) {
	root := newTestLayoutRoot(t)
	writeLayoutFile(t, root, "layout.xml",
		`<Layout tpl="&lt;html&gt;{{frag}}{{body}}&lt;/html&gt;">`+
			`<Content name="body" src="*.md" to="{FILENAME}.html"/>`+
			`<Fragment name="frag" src="frag.html" pipe=""/>`+
			`</Layout>`)
	writeLayoutFile(t, root, "frag.html", "FRAGTXT")

	lc := NewLayoutCache(root, newTestPipesRegistry(t), "layout.xml")
	key := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	entry, err := lc.Get(context.Background(), key, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	got := entry.ExecuteBytes(map[string]any{"body": "BODYTXT"})
	want := "<html>FRAGTXTBODYTXT</html>"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLayoutCache_Get_CachesCompiledEntryForSameKey(t *testing.T) {
	root := newTestLayoutRoot(t)
	writeLayoutFile(t, root, "layout.xml", `<Layout tpl="{{body}}"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`)

	lc := NewLayoutCache(root, newTestPipesRegistry(t), "layout.xml")
	key := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	entry1, err := lc.Get(context.Background(), key, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	entry2, err := lc.Get(context.Background(), key, nil)
	if err != nil {
		t.Fatalf("Get (2nd): %v", err)
	}
	if entry1 != entry2 {
		t.Fatalf("expected the second Get to return the cached compiled entry")
	}
}

func TestLayoutCache_Invalidate_ForcesRecompileAndReparse(t *testing.T) {
	root := newTestLayoutRoot(t)
	writeLayoutFile(t, root, "layout.xml", `<Layout tpl="{{body}}"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`)

	lc := NewLayoutCache(root, newTestPipesRegistry(t), "layout.xml")
	key := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	parsed1, err := lc.Parse(key)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	compiled1, err := lc.Get(context.Background(), key, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	lc.Invalidate(key)

	parsed2, err := lc.Parse(key)
	if err != nil {
		t.Fatalf("Parse (after invalidate): %v", err)
	}
	if parsed1 == parsed2 {
		t.Fatalf("expected Invalidate to force a fresh parse")
	}

	compiled2, err := lc.Get(context.Background(), key, nil)
	if err != nil {
		t.Fatalf("Get (after invalidate): %v", err)
	}
	if compiled1 == compiled2 {
		t.Fatalf("expected Invalidate to force a fresh compile")
	}
}

func TestLayoutCache_InvalidateAll_ClearsEveryCompiledEntry(t *testing.T) {
	root := newTestLayoutRoot(t)
	writeLayoutFile(t, root, "a.xml", `<Layout tpl="A:{{body}}"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`)
	writeLayoutFile(t, root, "b.xml", `<Layout tpl="B:{{body}}"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`)

	lc := NewLayoutCache(root, newTestPipesRegistry(t), "*.xml")
	keyA := LayoutKey{CtxPath: ".", LayoutPath: "a.xml"}
	keyB := LayoutKey{CtxPath: ".", LayoutPath: "b.xml"}

	entryA1, err := lc.Get(context.Background(), keyA, nil)
	if err != nil {
		t.Fatalf("Get a: %v", err)
	}
	entryB1, err := lc.Get(context.Background(), keyB, nil)
	if err != nil {
		t.Fatalf("Get b: %v", err)
	}

	lc.InvalidateAll()

	entryA2, err := lc.Get(context.Background(), keyA, nil)
	if err != nil {
		t.Fatalf("Get a (after invalidate all): %v", err)
	}
	entryB2, err := lc.Get(context.Background(), keyB, nil)
	if err != nil {
		t.Fatalf("Get b (after invalidate all): %v", err)
	}

	if entryA1 == entryA2 || entryB1 == entryB2 {
		t.Fatalf("expected InvalidateAll to force recompilation of both layouts")
	}
}

func TestLayoutCache_LayoutsInDir_GlobsWithinDir(t *testing.T) {
	root := newTestLayoutRoot(t)
	writeLayoutFile(t, root, "pages/index.layout.xml", `<Layout tpl="x"/>`)
	writeLayoutFile(t, root, "pages/notes.txt", "not a layout")

	lc := NewLayoutCache(root, newTestPipesRegistry(t), "*.layout.xml")

	found, err := lc.LayoutsInDir(context.Background(), "pages")
	if err != nil {
		t.Fatalf("LayoutsInDir: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly 1 layout, got %+v", found)
	}
	key, ok := found["pages/index.layout.xml"]
	if !ok {
		t.Fatalf("expected key pages/index.layout.xml, got %+v", found)
	}
	if key.CtxPath != "pages" || key.LayoutPath != "pages/index.layout.xml" {
		t.Fatalf("unexpected key: %+v", key)
	}
}

func TestLayoutCache_LayoutsInDir_NoMatches_ReturnsError(t *testing.T) {
	root := newTestLayoutRoot(t)
	writeLayoutFile(t, root, "pages/notes.txt", "not a layout")

	lc := NewLayoutCache(root, newTestPipesRegistry(t), "*.layout.xml")

	if _, err := lc.LayoutsInDir(context.Background(), "pages"); err == nil {
		t.Fatalf("expected error when no layout matches in dir")
	}
}

func TestLayoutCache_LayoutsForDir_FindsNearestAncestor(t *testing.T) {
	root := newTestLayoutRoot(t)
	writeLayoutFile(t, root, "layout.xml", `<Layout tpl="x"/>`)
	if err := os.MkdirAll(filepath.Join(root.Path(), "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}

	lc := NewLayoutCache(root, newTestPipesRegistry(t), "layout.xml")

	found, err := lc.LayoutsForDir(context.Background(), "sub")
	if err != nil {
		t.Fatalf("LayoutsForDir: %v", err)
	}
	key, ok := found["layout.xml"]
	if !ok {
		t.Fatalf("expected to find the ancestor's layout.xml, got %+v", found)
	}
	if key.LayoutPath != "layout.xml" {
		t.Fatalf("unexpected LayoutPath: %+v", key)
	}
}

func TestLayoutCache_LayoutsForDir_NoLayoutAnywhere_ReturnsError(t *testing.T) {
	root := newTestLayoutRoot(t)
	lc := NewLayoutCache(root, newTestPipesRegistry(t), "layout.xml")

	if _, err := lc.LayoutsForDir(context.Background(), "anything"); err == nil {
		t.Fatalf("expected error when no layout is found walking up from the target path")
	}
}
