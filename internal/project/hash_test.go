package project

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"craeft/internal/layout"
	"craeft/internal/pipeline"
)

// ---- fakes for the local interfaces declared in hash.go ----

type fakeFileHasher struct {
	mu     sync.Mutex
	hashes map[string]string
	errs   map[string]error
	calls  []string
}

func (f *fakeFileHasher) HashFile(path string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, path)
	if err, ok := f.errs[path]; ok {
		return "", err
	}
	if h, ok := f.hashes[path]; ok {
		return h, nil
	}
	return "", fmt.Errorf("no hash for %s", path)
}

// layoutWithFragment returns a real layout.Layout with one Content and one
// Fragment, so hashLayoutDeps has a fragment pipe to hash.
func layoutWithFragment(t *testing.T) *layout.Layout {
	t.Helper()
	xmlSrc := `<Layout tpl="&lt;html&gt;{{body}}{{frag}}&lt;/html&gt;">` +
		`<Content name="body" src="*.md" to="{FILENAME}.html"/>` +
		`<Fragment name="frag" src="frag.html" pipe="html"/>` +
		`</Layout>`
	lyt, err := layout.Parse("site", []byte(xmlSrc))
	if err != nil {
		t.Fatalf("parse test layout: %v", err)
	}
	return lyt
}

func TestHash_ComputeHash_ContentEntry_ChangesWithInputs(t *testing.T) {
	build := func(mutate func(sourceFS, schemeFS *fakeFileHasher, pipes *fakePipelineLoader, vars map[string]string)) string {
		lyt := layoutWithFragment(t)
		layoutEntry := &LayoutCacheEntry{layout: lyt, deps: []string{"frag.html"}}

		sourceFS := &fakeFileHasher{hashes: map[string]string{"about.md": "content-hash"}}
		schemeFS := &fakeFileHasher{hashes: map[string]string{
			"layout.xml": "layout-hash",
			"frag.html":  "frag-hash",
		}}
		pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{
			"md":   &fakePipe{hash: "md-pipe-hash"},
			"html": &fakePipe{hash: "html-pipe-hash"},
		}}
		vars := map[string]string{"x": "y"}

		if mutate != nil {
			mutate(sourceFS, schemeFS, pipes, vars)
		}

		layouts := &fakeLayoutLoader{lyt: lyt, entry: layoutEntry}
		hi := NewHashIndex(sourceFS, schemeFS, layouts, pipes)

		entry := &BuildEntry{
			Type: ContentType, SourceRoot: SourceTypeSource, SourcePath: "about.md",
			Pipeline: "md", LayoutKey: LayoutKey{LayoutPath: "layout.xml"}, ContentVars: vars,
		}

		hash, err := hi.ComputeHash(context.Background(), entry)
		if err != nil {
			t.Fatalf("ComputeHash: %v", err)
		}
		return hash
	}

	base := build(nil)
	if base == "" {
		t.Fatalf("expected a non-empty base hash")
	}
	if repeat := build(nil); repeat != base {
		t.Fatalf("expected identical inputs to produce the same hash, got %q vs %q", base, repeat)
	}

	cases := map[string]func(sourceFS, schemeFS *fakeFileHasher, pipes *fakePipelineLoader, vars map[string]string){
		"content changes": func(s, sc *fakeFileHasher, p *fakePipelineLoader, v map[string]string) {
			s.hashes["about.md"] = "content-hash-2"
		},
		"layout file changes": func(s, sc *fakeFileHasher, p *fakePipelineLoader, v map[string]string) {
			sc.hashes["layout.xml"] = "layout-hash-2"
		},
		"fragment dep changes": func(s, sc *fakeFileHasher, p *fakePipelineLoader, v map[string]string) {
			sc.hashes["frag.html"] = "frag-hash-2"
		},
		"content pipe changes": func(s, sc *fakeFileHasher, p *fakePipelineLoader, v map[string]string) {
			p.pipes["md"] = &fakePipe{hash: "md-pipe-hash-2"}
		},
		"fragment pipe changes": func(s, sc *fakeFileHasher, p *fakePipelineLoader, v map[string]string) {
			p.pipes["html"] = &fakePipe{hash: "html-pipe-hash-2"}
		},
		"vars change": func(s, sc *fakeFileHasher, p *fakePipelineLoader, v map[string]string) { v["x"] = "z" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			got := build(mutate)
			if got == base {
				t.Fatalf("expected hash to change when %s, but it stayed %q", name, got)
			}
		})
	}
}

func TestHash_ComputeHash_AssetEntry_HashesLayoutFileDirectly(t *testing.T) {
	layouts := &fakeLayoutLoader{}
	sourceFS := &fakeFileHasher{}
	schemeFS := &fakeFileHasher{hashes: map[string]string{
		"style.css":  "asset-content-hash",
		"layout.xml": "layout-hash",
	}}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"css": &fakePipe{hash: "css-pipe-hash"}}}

	hi := NewHashIndex(sourceFS, schemeFS, layouts, pipes)

	entry := &BuildEntry{
		Type: AssetType, SourceRoot: SourceTypeScheme, SourcePath: "style.css",
		Pipeline: "css", LayoutKey: LayoutKey{LayoutPath: "layout.xml"},
	}

	hash, err := hi.ComputeHash(context.Background(), entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if layouts.getCalls != 0 {
		t.Fatalf("expected layouts.Get not to be called for asset entries (only the layout file itself is hashed), got %d calls", layouts.getCalls)
	}

	// An asset entry's hash must change when its governing layout changes,
	// so the CAS cache invalidates correctly.
	schemeFS.hashes["layout.xml"] = "layout-hash-changed"
	changedHash, err := hi.ComputeHash(context.Background(), entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changedHash == hash {
		t.Fatalf("expected an asset entry's hash to change when its layout file changes, got the same hash %q both times", hash)
	}
}

func TestHash_ComputeHash_NoLayoutKey_SkipsLayoutHashing(t *testing.T) {
	sourceFS := &fakeFileHasher{hashes: map[string]string{"about.md": "content-hash"}}
	schemeFS := &fakeFileHasher{}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"md": &fakePipe{hash: "md-pipe-hash"}}}
	layouts := &fakeLayoutLoader{}

	hi := NewHashIndex(sourceFS, schemeFS, layouts, pipes)
	entry := &BuildEntry{Type: ContentType, SourceRoot: SourceTypeSource, SourcePath: "about.md", Pipeline: "md"}

	if _, err := hi.ComputeHash(context.Background(), entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(schemeFS.calls) != 0 {
		t.Fatalf("expected schemeFS not to be touched when entry has no layout key, got %v", schemeFS.calls)
	}
}

func TestHash_ComputeHash_UnknownSourceRoot_ReturnsError(t *testing.T) {
	hi := NewHashIndex(&fakeFileHasher{}, &fakeFileHasher{}, &fakeLayoutLoader{}, &fakePipelineLoader{})
	entry := &BuildEntry{SourceRoot: SourceTypeOutput, SourcePath: "x"}

	if _, err := hi.ComputeHash(context.Background(), entry); err == nil {
		t.Fatalf("expected error for unknown source root")
	}
}

func TestHash_ComputeHash_ContentFileReadError_IsWrapped(t *testing.T) {
	readErr := errors.New("disk error")
	sourceFS := &fakeFileHasher{errs: map[string]error{"about.md": readErr}}
	hi := NewHashIndex(sourceFS, &fakeFileHasher{}, &fakeLayoutLoader{}, &fakePipelineLoader{})
	entry := &BuildEntry{Type: ContentType, SourceRoot: SourceTypeSource, SourcePath: "about.md"}

	_, err := hi.ComputeHash(context.Background(), entry)
	if err == nil || !errors.Is(err, readErr) {
		t.Fatalf("expected wrapped read error, got %v", err)
	}
}

func TestHash_ComputeHash_UnknownPipeline_ProducesEmptyPipelineHashWithoutError(t *testing.T) {
	sourceFS := &fakeFileHasher{hashes: map[string]string{"about.md": "content-hash"}}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{}}
	hi := NewHashIndex(sourceFS, &fakeFileHasher{}, &fakeLayoutLoader{}, pipes)
	entry := &BuildEntry{Type: ContentType, SourceRoot: SourceTypeSource, SourcePath: "about.md", Pipeline: "missing"}

	hash, err := hi.ComputeHash(context.Background(), entry)
	if err != nil {
		t.Fatalf("unexpected error for unknown pipeline: %v", err)
	}
	if hash == "" {
		t.Fatalf("expected a non-empty combined hash even with an empty pipeline hash")
	}
}

func TestHash_HashAll_EmptyGraph_NoError(t *testing.T) {
	g := NewDependencyGraph()
	hi := NewHashIndex(&fakeFileHasher{}, &fakeFileHasher{}, &fakeLayoutLoader{}, &fakePipelineLoader{})
	if err := hi.HashAll(context.Background(), g); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHash_HashAll_UpdatesGraphEntries(t *testing.T) {
	g := NewDependencyGraph()
	e1 := contentEntry("a.md", "body")
	e2 := contentEntry("b.md", "body")
	g.AddNode(NewEntryNode(e1))
	g.AddNode(NewEntryNode(e2))

	sourceFS := &fakeFileHasher{hashes: map[string]string{"a.md": "hash-a", "b.md": "hash-b"}}
	hi := NewHashIndex(sourceFS, &fakeFileHasher{}, &fakeLayoutLoader{}, &fakePipelineLoader{pipes: map[string]pipeline.Pipe{}})

	if err := hi.HashAll(context.Background(), g); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got1, _ := g.EntryByID(e1.ID)
	got2, _ := g.EntryByID(e2.ID)
	if got1.Hash == "" || got2.Hash == "" {
		t.Fatalf("expected both entries to have a hash set, got %q and %q", got1.Hash, got2.Hash)
	}
	if got1.Hash == got2.Hash {
		t.Fatalf("expected different content to produce different hashes")
	}
}

func TestHash_HashAll_AggregatesPerEntryErrors(t *testing.T) {
	g := NewDependencyGraph()
	okEntry := contentEntry("ok.md", "body")
	badEntry := contentEntry("bad.md", "body")
	g.AddNode(NewEntryNode(okEntry))
	g.AddNode(NewEntryNode(badEntry))

	sourceFS := &fakeFileHasher{
		hashes: map[string]string{"ok.md": "hash-ok"},
		errs:   map[string]error{"bad.md": errors.New("boom")},
	}
	hi := NewHashIndex(sourceFS, &fakeFileHasher{}, &fakeLayoutLoader{}, &fakePipelineLoader{pipes: map[string]pipeline.Pipe{}})

	err := hi.HashAll(context.Background(), g)
	if err == nil {
		t.Fatalf("expected error for the failing entry")
	}

	gotOK, _ := g.EntryByID(okEntry.ID)
	gotBad, _ := g.EntryByID(badEntry.ID)
	if gotOK.Hash == "" {
		t.Fatalf("expected the successful entry to still get a hash")
	}
	if gotBad.Hash != "" {
		t.Fatalf("expected the failing entry to have no hash, got %q", gotBad.Hash)
	}
}
