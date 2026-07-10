package server

import (
	"testing"

	"craeft/internal/project"
)

type fakeReloader struct {
	reloadCalls   int
	navigateCalls []string
}

func (f *fakeReloader) ReloadPage()           { f.reloadCalls++ }
func (f *fakeReloader) NavigateTo(url string) { f.navigateCalls = append(f.navigateCalls, url) }

func TestGroupByHandler_GroupsPathsByTypeAndMod(t *testing.T) {
	changes := []FileChange{
		{Path: "a.md", Type: FileTypeContent, Mod: ModTypeModified},
		{Path: "b.md", Type: FileTypeContent, Mod: ModTypeModified},
		{Path: "c.md", Type: FileTypeContent, Mod: ModTypeCreated},
		{Path: "style.css", Type: FileTypeSchemeAsset, Mod: ModTypeModified},
	}

	grouped := groupByHandler(changes)

	contentModified := grouped[changeKey{FileTypeContent, ModTypeModified}]
	if len(contentModified) != 2 || contentModified[0] != "a.md" || contentModified[1] != "b.md" {
		t.Fatalf("expected a.md and b.md grouped under (content, modified), got %v", contentModified)
	}
	if len(grouped[changeKey{FileTypeContent, ModTypeCreated}]) != 1 {
		t.Fatalf("expected c.md in its own group")
	}
	if len(grouped[changeKey{FileTypeSchemeAsset, ModTypeModified}]) != 1 {
		t.Fatalf("expected style.css in its own group")
	}
	if len(grouped) != 3 {
		t.Fatalf("expected exactly 3 groups, got %d", len(grouped))
	}
}

func TestPathsFromChanges_ExtractsPathsInOrder(t *testing.T) {
	changes := []FileChange{
		{Path: "a.md"},
		{Path: "b.md"},
	}
	got := pathsFromChanges(changes)
	if len(got) != 2 || got[0] != "a.md" || got[1] != "b.md" {
		t.Fatalf("unexpected paths: %v", got)
	}
}

// TestDebouncer_Notify_StructuralChange_TriggersFullReload models a layout
// or fragment edit: since it can affect the whole page structure (or
// multiple pages), the browser should do a full reload rather than a
// targeted navigation.
func TestDebouncer_Notify_StructuralChange_TriggersFullReload(t *testing.T) {
	p, _ := newTestServerProject(t)
	hr := &fakeReloader{}
	d := &Debouncer{project: p, hr: hr}

	changes := []FileChange{{Path: "layout.xml", Type: FileTypeLayout, Mod: ModTypeModified}}
	dirty := []*project.BuildEntry{{ID: "about.md:body", Type: project.ContentType}}

	d.notify(changes, dirty)

	if hr.reloadCalls != 1 {
		t.Fatalf("expected exactly 1 full reload, got %d", hr.reloadCalls)
	}
	if len(hr.navigateCalls) != 0 {
		t.Fatalf("expected no navigation for a structural change, got %v", hr.navigateCalls)
	}
}

// TestDebouncer_Notify_SingleContentEdit_NavigatesToRebuiltPage models the
// most common editing loop: a single markdown/content file is saved, so the
// browser should just navigate to that page's freshly built URL.
func TestDebouncer_Notify_SingleContentEdit_NavigatesToRebuiltPage(t *testing.T) {
	p, _ := newTestServerProject(t)
	p.Manifest.Set("about.md:body", project.ManifestEntry{Hash: "h1", OutputPath: "about.html"})

	hr := &fakeReloader{}
	d := &Debouncer{project: p, hr: hr}

	changes := []FileChange{{Path: "about.md", Type: FileTypeContent, Mod: ModTypeModified}}
	dirty := []*project.BuildEntry{{ID: "about.md:body", Type: project.ContentType}}

	d.notify(changes, dirty)

	if len(hr.navigateCalls) != 1 || hr.navigateCalls[0] != "/about.html" {
		t.Fatalf("expected navigation to /about.html, got %v", hr.navigateCalls)
	}
	if hr.reloadCalls != 0 {
		t.Fatalf("expected no full reload when a single page can be targeted, got %d", hr.reloadCalls)
	}
}

// TestDebouncer_Notify_MultipleContentEdits_FallsBackToFullReload models
// saving several content files at once (e.g. a bulk find-and-replace):
// there's no single page to navigate to, so it should fall back to a full
// reload.
func TestDebouncer_Notify_MultipleContentEdits_FallsBackToFullReload(t *testing.T) {
	p, _ := newTestServerProject(t)
	hr := &fakeReloader{}
	d := &Debouncer{project: p, hr: hr}

	changes := []FileChange{
		{Path: "a.md", Type: FileTypeContent, Mod: ModTypeModified},
		{Path: "b.md", Type: FileTypeContent, Mod: ModTypeModified},
	}
	dirty := []*project.BuildEntry{
		{ID: "a.md:body", Type: project.ContentType},
		{ID: "b.md:body", Type: project.ContentType},
	}

	d.notify(changes, dirty)

	if hr.reloadCalls != 1 {
		t.Fatalf("expected a full reload for multiple dirty content entries, got %d", hr.reloadCalls)
	}
	if len(hr.navigateCalls) != 0 {
		t.Fatalf("expected no navigation, got %v", hr.navigateCalls)
	}
}

// TestDebouncer_Notify_ContentEntryMissingFromManifest_FallsBackToFullReload
// covers the edge case where a content entry was rebuilt but somehow has no
// manifest record yet (e.g. build error before the manifest write) -- it
// must not crash and should just fall back to a full reload.
func TestDebouncer_Notify_ContentEntryMissingFromManifest_FallsBackToFullReload(t *testing.T) {
	p, _ := newTestServerProject(t)
	hr := &fakeReloader{}
	d := &Debouncer{project: p, hr: hr}

	changes := []FileChange{{Path: "about.md", Type: FileTypeContent, Mod: ModTypeModified}}
	dirty := []*project.BuildEntry{{ID: "about.md:body", Type: project.ContentType}}

	d.notify(changes, dirty)

	if hr.reloadCalls != 1 {
		t.Fatalf("expected a full reload fallback when the manifest has no entry, got %d", hr.reloadCalls)
	}
}

// TestDebouncer_HandleBatch_ContentModified_RebuildsAffectedPage is an
// end-to-end test of the real save-triggers-rebuild flow: classify a raw
// filesystem change, dispatch it through the real registry/handlers, and
// confirm the incremental rebuild actually re-renders the changed page on
// disk -- the thing a user watching `craeft serve` actually cares about.
func TestDebouncer_HandleBatch_ContentModified_RebuildsAffectedPage(t *testing.T) {
	p, cfg := newTestServerProject(t)
	writeServerSourceFile(t, cfg, "about.md", "hello world")

	if err := p.Build(t.Context()); err != nil {
		t.Fatalf("initial Build: %v", err)
	}

	writeServerSourceFile(t, cfg, "about.md", "updated content")

	classifier := NewFileClassifier(p, cfg.SourcePath, cfg.SchemePath, cfg.PipesPath, cfg.OutputPath, p.Ignore)
	registry := NewEventRegistry()
	NewHandlers(p).RegisterAll(registry)

	d := NewDebouncer(DebouncerConfig{}, p, registry, classifier, nil)

	err := d.handleBatch(t.Context(), []RawChange{
		{Path: cfg.SourcePath + "/about.md", Mod: ModTypeModified},
	})
	if err != nil {
		t.Fatalf("handleBatch: %v", err)
	}

	data, err := p.Output.ReadFile("about.html")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "<html>updated content</html>" {
		t.Fatalf("expected the rebuild to reflect the edit, got %q", data)
	}
}
