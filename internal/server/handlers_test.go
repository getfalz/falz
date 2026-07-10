package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"craeft/internal/project"
)

// newTestServerProject sets up a minimal, valid project on disk (empty
// pipes dir, a scheme dir with a single layout wrapping "body" content in
// <html>...</html>, and an empty source/output dir) and opens it.
func newTestServerProject(t *testing.T) (*project.Project, *project.CraeftConfig) {
	t.Helper()
	base := t.TempDir()
	sourcePath := filepath.Join(base, "src")
	schemePath := filepath.Join(base, "scheme")
	outputPath := filepath.Join(base, "dist")
	pipesPath := filepath.Join(base, "pipes")

	for _, d := range []string{sourcePath, schemePath, pipesPath} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	layoutXML := `<Layout tpl="&lt;html&gt;{{body}}&lt;/html&gt;"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`
	if err := os.WriteFile(filepath.Join(schemePath, "layout.xml"), []byte(layoutXML), 0o644); err != nil {
		t.Fatalf("write layout: %v", err)
	}

	cfg := &project.CraeftConfig{
		OutputPath: outputPath,
		SourcePath: sourcePath,
		SchemePath: schemePath,
		PipesPath:  pipesPath,
		Layout:     "layout.xml",
		Build:      project.BuildConfig{Concurrency: 1},
	}

	p, err := project.New(cfg)
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	return p, cfg
}

func writeServerSourceFile(t *testing.T, cfg *project.CraeftConfig, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cfg.SourcePath, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write source file %s: %v", name, err)
	}
}

// TestRehashAndMark_NeverHashedEntry_DoesNotPanic is a regression test for a
// crash when an entry's previous Hash is empty (e.g. a freshly scanned entry
// that hasn't been hashed yet): rehashAndMark used to slice oldHash[:8]
// unconditionally, which panics (slice bounds out of range) whenever
// len(oldHash) < 8 -- including the empty-string case.
func TestRehashAndMark_NeverHashedEntry_DoesNotPanic(t *testing.T) {
	p, cfg := newTestServerProject(t)
	writeServerSourceFile(t, cfg, "about.md", "hello world")

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build: %v", err)
	}

	entries := p.Graph().AllEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	id := entries[0].ID

	// Simulate an entry that has never been hashed.
	if err := p.Graph().UpdateEntry(id, func(e *project.BuildEntry) error {
		e.Hash = ""
		return nil
	}); err != nil {
		t.Fatalf("UpdateEntry: %v", err)
	}

	h := NewHandlers(p)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("rehashAndMark panicked on an empty previous hash: %v", r)
		}
	}()
	h.rehashAndMark(context.Background(), id)

	entry, ok := p.Graph().EntryByID(id)
	if !ok || entry.Hash == "" {
		t.Fatalf("expected the entry to be rehashed, got %+v (ok=%v)", entry, ok)
	}
}

// TestHandleLayoutModified_PicksUpNewContentPattern is a regression test for
// a bug where modifying a layout to add a new Content/Asset pattern didn't
// rescan the affected directory: handleLayoutModified only invalidated and
// rehashed entries *already in the graph*, via invalidateAndRehash. A
// PartialScan of the layout's directory was added so newly-matching source
// files are picked up as soon as the layout changes, instead of only on the
// next full rebuild.
func TestHandleLayoutModified_PicksUpNewContentPattern(t *testing.T) {
	p, cfg := newTestServerProject(t)
	writeServerSourceFile(t, cfg, "about.md", "hello world")

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Add a new Content pattern to the layout, and a source file that
	// matches only the new pattern.
	newLayoutXML := `<Layout tpl="&lt;html&gt;{{body}}{{excerpt}}&lt;/html&gt;">` +
		`<Content name="body" src="*.md" to="{FILENAME}.html"/>` +
		`<Content name="excerpt" src="*.txt" to="{FILENAME}.txt"/>` +
		`</Layout>`
	if err := os.WriteFile(filepath.Join(cfg.SchemePath, "layout.xml"), []byte(newLayoutXML), 0o644); err != nil {
		t.Fatalf("write updated layout: %v", err)
	}
	writeServerSourceFile(t, cfg, "notes.txt", "some notes")

	h := NewHandlers(p)
	if err := h.handleLayoutModified(context.Background(), []string{"layout.xml"}); err != nil {
		t.Fatalf("handleLayoutModified: %v", err)
	}

	if _, ok := p.Graph().EntryByID("notes.txt:excerpt"); !ok {
		t.Fatalf("expected the new Content pattern to be picked up and notes.txt:excerpt added to the graph")
	}
}
