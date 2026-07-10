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
		ConfigFile: filepath.Join(base, ".craeft", "config.yaml"),
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

// TestHandleContentCreated_NewFileMatchingPattern_AddsHashedDirtyEntry
// models the most common watch-mode event: the user adds a brand-new
// content file while the server is running, and it should show up in the
// graph, already hashed, and marked dirty for the next rebuild.
func TestHandleContentCreated_NewFileMatchingPattern_AddsHashedDirtyEntry(t *testing.T) {
	p, cfg := newTestServerProject(t)
	h := NewHandlers(p)

	writeServerSourceFile(t, cfg, "about.md", "hello world")

	if err := h.handleContentCreated(context.Background(), []string{"about.md"}); err != nil {
		t.Fatalf("handleContentCreated: %v", err)
	}

	entry, ok := p.Graph().EntryByID("about.md:body")
	if !ok {
		t.Fatalf("expected about.md:body to be added to the graph")
	}
	if entry.Hash == "" {
		t.Fatalf("expected the new entry to already be hashed")
	}
	if !entry.Dirty {
		t.Fatalf("expected the new entry to be marked dirty for the next build")
	}
}

// TestHandleContentCreated_UnmatchedFile_IsIgnored models adding a file
// that isn't picked up by any layout's Content pattern (e.g. a stray
// .DS_Store or a draft with the wrong extension): it must not be added to
// the graph or cause an error.
func TestHandleContentCreated_UnmatchedFile_IsIgnored(t *testing.T) {
	p, cfg := newTestServerProject(t)
	h := NewHandlers(p)

	writeServerSourceFile(t, cfg, "notes.txt", "not markdown")

	if err := h.handleContentCreated(context.Background(), []string{"notes.txt"}); err != nil {
		t.Fatalf("handleContentCreated: %v", err)
	}

	if len(p.Graph().AllEntries()) != 0 {
		t.Fatalf("expected no entries for an unmatched file, got %+v", p.Graph().AllEntries())
	}
}

// TestHandleContentModified_ThenIncrementalBuild_UpdatesOutput models the
// full real loop: build once, edit the file, get notified, and confirm the
// incremental rebuild actually reflects the edit on disk.
func TestHandleContentModified_ThenIncrementalBuild_UpdatesOutput(t *testing.T) {
	p, cfg := newTestServerProject(t)
	writeServerSourceFile(t, cfg, "about.md", "hello world")

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("initial Build: %v", err)
	}

	writeServerSourceFile(t, cfg, "about.md", "hello universe")

	h := NewHandlers(p)
	if err := h.handleContentModified(context.Background(), []string{"about.md"}); err != nil {
		t.Fatalf("handleContentModified: %v", err)
	}

	entry, ok := p.Graph().EntryByID("about.md:body")
	if !ok || !entry.Dirty {
		t.Fatalf("expected the entry to be marked dirty after the edit, got %+v (ok=%v)", entry, ok)
	}

	if _, err := p.IncrementalBuild(context.Background()); err != nil {
		t.Fatalf("IncrementalBuild: %v", err)
	}

	data, err := p.Output.ReadFile("about.html")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "<html>hello universe</html>" {
		t.Fatalf("expected the rebuild to reflect the edit, got %q", data)
	}
}

// TestHandleContentDeleted_RemovesEntryAndManifestRecord models deleting a
// content file while the server is running: both the graph entry and its
// manifest record must be cleaned up so a stale output isn't served.
func TestHandleContentDeleted_RemovesEntryAndManifestRecord(t *testing.T) {
	p, cfg := newTestServerProject(t)
	writeServerSourceFile(t, cfg, "about.md", "hello world")

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("initial Build: %v", err)
	}
	if err := os.Remove(filepath.Join(cfg.SourcePath, "about.md")); err != nil {
		t.Fatalf("remove source file: %v", err)
	}

	h := NewHandlers(p)
	if err := h.handleContentDeleted(context.Background(), []string{"about.md"}); err != nil {
		t.Fatalf("handleContentDeleted: %v", err)
	}

	if _, ok := p.Graph().EntryByID("about.md:body"); ok {
		t.Fatalf("expected the entry to be removed from the graph")
	}
	if _, ok := p.Manifest.Get("about.md:body"); ok {
		t.Fatalf("expected the manifest record to be removed too")
	}
}

// TestHandleConfigModified_NeverEnablesCASWithoutABackend is a regression
// test for a bug where handleConfigModified copied the freshly reloaded
// BuildConfig (including UseCAS) onto the live project without creating or
// refreshing the project's CAS backend to match. A project that starts
// with CAS disabled has p.CAS == nil; a config reload flips UseCAS on even
// with a completely absent config.yaml, since BuildConfig.UseCAS defaults
// to "true". The combination (UseCAS=true, CAS==nil) used to be reachable
// and is invalid: build.go passes the (typed-nil) *project.CAS through the
// artifactCache interface, so `cache != nil` reads true even though the
// pointer is nil, and the next incremental build panics inside a worker
// goroutine. handleConfigModified now creates the CAS backend when it's
// missing and UseCAS is enabled (and closes it when disabled).
func TestHandleConfigModified_NeverEnablesCASWithoutABackend(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	p, cfg := newTestServerProject(t)
	writeServerSourceFile(t, cfg, "about.md", "hello world")

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("initial Build: %v", err)
	}
	if p.CAS != nil {
		t.Fatalf("test setup assumption broken: expected CAS to start disabled")
	}

	h := NewHandlers(p)
	if err := h.handleConfigModified(context.Background(), nil); err != nil {
		t.Fatalf("handleConfigModified: %v", err)
	}
	if p.CAS != nil {
		t.Cleanup(func() { _ = p.CAS.Close() })
	}

	if p.Config.Build.UseCAS && p.CAS == nil {
		t.Fatalf("BUG: config reload enabled UseCAS but left the CAS backend (p.CAS) nil -- the next incremental build will panic")
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
