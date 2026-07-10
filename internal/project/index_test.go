package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"

	"craeft/internal/layout"
)

// ---- fakes for the local interfaces declared in index.go ----

type fakeSourceTree struct {
	fsys        fs.FS
	globResults map[string][]string
	globErrs    map[string]error
}

func (f *fakeSourceTree) FS() fs.FS { return f.fsys }

func (f *fakeSourceTree) GlobSrc(base, src string) ([]string, error) {
	key := base + "::" + src
	if err, ok := f.globErrs[key]; ok {
		return nil, err
	}
	return f.globResults[key], nil
}

type fakeSchemeTree struct {
	findResults map[string][]string
	findErrs    map[string]error
	globResults map[string][]string
	globErrs    map[string]error
}

func (f *fakeSchemeTree) FindFile(targetPath, pattern string) ([]string, error) {
	key := targetPath + "::" + pattern
	if err, ok := f.findErrs[key]; ok {
		return nil, err
	}
	if res, ok := f.findResults[key]; ok {
		return res, nil
	}
	return nil, fmt.Errorf("not found: %s", key)
}

func (f *fakeSchemeTree) GlobSrc(base, src string) ([]string, error) {
	key := base + "::" + src
	if err, ok := f.globErrs[key]; ok {
		return nil, err
	}
	return f.globResults[key], nil
}

type fakeLayoutFinder struct {
	forDir     map[string]map[string]LayoutKey
	forDirErrs map[string]error
	inDir      map[string]map[string]LayoutKey
	inDirErrs  map[string]error
	parse      map[string]*layout.Layout
	parseErrs  map[string]error
}

func (f *fakeLayoutFinder) Parse(key LayoutKey) (*layout.Layout, error) {
	if err, ok := f.parseErrs[key.LayoutPath]; ok {
		return nil, err
	}
	if lyt, ok := f.parse[key.LayoutPath]; ok {
		return lyt, nil
	}
	return nil, fmt.Errorf("no layout for %s", key.LayoutPath)
}

func (f *fakeLayoutFinder) LayoutsForDir(ctx context.Context, path string) (map[string]LayoutKey, error) {
	if err, ok := f.forDirErrs[path]; ok {
		return nil, err
	}
	return f.forDir[path], nil
}

func (f *fakeLayoutFinder) LayoutsInDir(ctx context.Context, path string) (map[string]LayoutKey, error) {
	if err, ok := f.inDirErrs[path]; ok {
		return nil, err
	}
	return f.inDir[path], nil
}

func parseTestLayout(t *testing.T, path, xmlSrc string) *layout.Layout {
	t.Helper()
	lyt, err := layout.Parse(path, []byte(xmlSrc))
	if err != nil {
		t.Fatalf("parse layout: %v", err)
	}
	return lyt
}

func TestIndex_Scan_BuildsContentAndAssetEntriesWithEdges(t *testing.T) {
	fsys := fstest.MapFS{"about.md": &fstest.MapFile{Data: []byte("hi")}}

	xmlSrc := `<Layout src="layout.html">` +
		`<Fragment name="frag" src="frag.html" pipe="html"/>` +
		`<Content name="body" src="*.md" to="{FILENAME}.html"/>` +
		`<Asset name="style" src="*.css" to="{FILENAME}.css"/>` +
		`</Layout>`
	lyt := parseTestLayout(t, ".", xmlSrc)
	lKey := LayoutKey{CtxPath: ".", LayoutPath: "layout.html"}

	layouts := &fakeLayoutFinder{
		forDir: map[string]map[string]LayoutKey{".": {"layout.html": lKey}},
		parse:  map[string]*layout.Layout{"layout.html": lyt},
	}
	source := &fakeSourceTree{
		fsys:        fsys,
		globResults: map[string][]string{".::*.md": {"about.md"}},
	}
	scheme := &fakeSchemeTree{
		findResults: map[string][]string{
			".::frag.html":   {"frag.html"},
			".::layout.html": {"layout.html"},
		},
		globResults: map[string][]string{".::*.css": {"style.css"}},
	}

	graph := NewDependencyGraph()
	si := NewSourceIndex(source, scheme, layouts, NewIgnoreList(nil, nil))

	if err := si.Scan(context.Background(), graph); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	contentEntry, ok := graph.EntryByID("about.md:body")
	if !ok {
		t.Fatalf("expected content entry about.md:body, entries: %+v", graph.AllEntries())
	}
	if contentEntry.Type != ContentType || contentEntry.SourceRoot != SourceTypeSource || contentEntry.BaseDir != "." {
		t.Fatalf("unexpected content entry: %+v", contentEntry)
	}

	assetEntry, ok := graph.EntryByID("asset:scheme:style.css:style")
	if !ok {
		t.Fatalf("expected asset entry asset:scheme:style.css:style, entries: %+v", graph.AllEntries())
	}
	if assetEntry.Type != AssetType || assetEntry.SourceRoot != SourceTypeScheme {
		t.Fatalf("unexpected asset entry: %+v", assetEntry)
	}

	lytNode := NewLayoutNode(lKey)
	deps := graph.DependenciesOf(lytNode)
	var haveFragment, haveTemplate bool
	for _, d := range deps {
		switch d.Kind() {
		case NodeKindFragment:
			haveFragment = true
		case NodeKindTemplate:
			haveTemplate = true
		}
	}
	if !haveFragment || !haveTemplate {
		t.Fatalf("expected layout to depend on its fragment and template, got %+v", deps)
	}

	fragDeps := graph.DependenciesOf(NewFragmentNode("frag.html"))
	if len(fragDeps) != 1 || fragDeps[0].Kind() != NodeKindPipeline {
		t.Fatalf("expected fragment to depend on its pipeline, got %+v", fragDeps)
	}
}

func TestIndex_Scan_DirWithoutLayout_SkippedWithoutError(t *testing.T) {
	fsys := fstest.MapFS{"about.md": &fstest.MapFile{Data: []byte("hi")}}

	layouts := &fakeLayoutFinder{
		forDirErrs: map[string]error{".": errors.New("no layout found")},
	}
	source := &fakeSourceTree{fsys: fsys}
	scheme := &fakeSchemeTree{}

	graph := NewDependencyGraph()
	si := NewSourceIndex(source, scheme, layouts, NewIgnoreList(nil, nil))

	if err := si.Scan(context.Background(), graph); err != nil {
		t.Fatalf("expected no error when a directory has no layout, got %v", err)
	}
	if len(graph.AllEntries()) != 0 {
		t.Fatalf("expected no entries, got %+v", graph.AllEntries())
	}
}

func TestIndex_Scan_IgnoredDirectory_IsSkipped(t *testing.T) {
	fsys := fstest.MapFS{
		"ignored/about.md": &fstest.MapFile{Data: []byte("hi")},
	}

	layouts := &fakeLayoutFinder{
		forDirErrs: map[string]error{
			".":       errors.New("no layout at root"),
			"ignored": errors.New("should never be scanned"),
		},
	}
	source := &fakeSourceTree{fsys: fsys}
	scheme := &fakeSchemeTree{}

	graph := NewDependencyGraph()
	si := NewSourceIndex(source, scheme, layouts, NewIgnoreList([]string{"ignored/**"}, nil))

	if err := si.Scan(context.Background(), graph); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.AllEntries()) != 0 {
		t.Fatalf("expected no entries for an ignored directory, got %+v", graph.AllEntries())
	}
}

func TestIndex_Scan_FragmentLookupError_AbortsScan(t *testing.T) {
	fsys := fstest.MapFS{"about.md": &fstest.MapFile{Data: []byte("hi")}}

	xmlSrc := `<Layout tpl="x"><Fragment name="frag" src="frag.html" pipe="html"/></Layout>`
	lyt := parseTestLayout(t, ".", xmlSrc)
	lKey := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	layouts := &fakeLayoutFinder{
		forDir: map[string]map[string]LayoutKey{".": {"layout.xml": lKey}},
		parse:  map[string]*layout.Layout{"layout.xml": lyt},
	}
	source := &fakeSourceTree{fsys: fsys}
	scheme := &fakeSchemeTree{
		findErrs: map[string]error{".::frag.html": errors.New("fragment file missing")},
	}

	graph := NewDependencyGraph()
	si := NewSourceIndex(source, scheme, layouts, NewIgnoreList(nil, nil))

	if err := si.Scan(context.Background(), graph); err == nil {
		t.Fatalf("expected Scan to propagate the fragment lookup error")
	}
}

func TestIndex_PartialScan_StopsAtSubdirWithOwnLayout(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md":     &fstest.MapFile{Data: []byte("root")},
		"sub/b.md": &fstest.MapFile{Data: []byte("nested")},
	}

	xmlSrc := `<Layout tpl="x"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`
	lyt := parseTestLayout(t, ".", xmlSrc)
	lKey := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	layouts := &fakeLayoutFinder{
		forDir: map[string]map[string]LayoutKey{".": {"layout.xml": lKey}},
		parse:  map[string]*layout.Layout{"layout.xml": lyt},
		inDir: map[string]map[string]LayoutKey{
			"sub": {"sub/layout.xml": {CtxPath: "sub", LayoutPath: "sub/layout.xml"}},
		},
	}
	source := &fakeSourceTree{
		fsys:        fsys,
		globResults: map[string][]string{".::*.md": {"a.md"}},
	}
	scheme := &fakeSchemeTree{}

	graph := NewDependencyGraph()
	si := NewSourceIndex(source, scheme, layouts, NewIgnoreList(nil, nil))

	if err := si.PartialScan(context.Background(), graph, "."); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := graph.AllEntries()
	if len(entries) != 1 || entries[0].SourcePath != "a.md" {
		t.Fatalf("expected only the root entry to be scanned, got %+v", entries)
	}
}

func TestIndex_AddFile_MatchesContentPattern(t *testing.T) {
	xmlSrc := `<Layout tpl="x"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`
	lyt := parseTestLayout(t, ".", xmlSrc)
	lKey := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	layouts := &fakeLayoutFinder{
		forDir: map[string]map[string]LayoutKey{".": {"layout.xml": lKey}},
		parse:  map[string]*layout.Layout{"layout.xml": lyt},
	}
	si := NewSourceIndex(&fakeSourceTree{}, &fakeSchemeTree{}, layouts, NewIgnoreList(nil, nil))

	graph := NewDependencyGraph()
	entries, matched := si.AddFile(context.Background(), graph, "about.md", SourceTypeSource)
	if !matched || len(entries) != 1 {
		t.Fatalf("expected a single matched entry, got %+v (matched=%v)", entries, matched)
	}
	if entries[0].Type != ContentType || entries[0].TagName != "body" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestIndex_AddFile_MatchesContentAssetPattern(t *testing.T) {
	xmlSrc := `<Layout tpl="x"><Content name="body" src="*.md" to="{FILENAME}.html">` +
		`<Asset name="thumb" src="*.png" to="{FILENAME}.png"/>` +
		`</Content></Layout>`
	lyt := parseTestLayout(t, ".", xmlSrc)
	lKey := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	layouts := &fakeLayoutFinder{
		forDir: map[string]map[string]LayoutKey{".": {"layout.xml": lKey}},
		parse:  map[string]*layout.Layout{"layout.xml": lyt},
	}
	si := NewSourceIndex(&fakeSourceTree{}, &fakeSchemeTree{}, layouts, NewIgnoreList(nil, nil))

	graph := NewDependencyGraph()
	entries, matched := si.AddFile(context.Background(), graph, "hero.png", SourceTypeSource)
	if !matched || len(entries) != 1 {
		t.Fatalf("expected a single matched entry, got %+v (matched=%v)", entries, matched)
	}
	if entries[0].Type != AssetType || entries[0].TagName != "thumb" || entries[0].SourceRoot != SourceTypeSource {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestIndex_AddFile_MatchesSchemeAsset(t *testing.T) {
	xmlSrc := `<Layout tpl="x"><Asset name="style" src="*.css" to="{FILENAME}.css"/></Layout>`
	lyt := parseTestLayout(t, ".", xmlSrc)
	lKey := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	layouts := &fakeLayoutFinder{
		forDir: map[string]map[string]LayoutKey{".": {"layout.xml": lKey}},
		parse:  map[string]*layout.Layout{"layout.xml": lyt},
	}
	si := NewSourceIndex(&fakeSourceTree{}, &fakeSchemeTree{}, layouts, NewIgnoreList(nil, nil))

	graph := NewDependencyGraph()
	entries, matched := si.AddFile(context.Background(), graph, "style.css", SourceTypeScheme)
	if !matched || len(entries) != 1 {
		t.Fatalf("expected a single matched entry, got %+v (matched=%v)", entries, matched)
	}
	if entries[0].Type != AssetType || entries[0].SourceRoot != SourceTypeScheme || entries[0].TagName != "style" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestIndex_AddFile_NoMatch_ReturnsFalse(t *testing.T) {
	xmlSrc := `<Layout tpl="x"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`
	lyt := parseTestLayout(t, ".", xmlSrc)
	lKey := LayoutKey{CtxPath: ".", LayoutPath: "layout.xml"}

	layouts := &fakeLayoutFinder{
		forDir: map[string]map[string]LayoutKey{".": {"layout.xml": lKey}},
		parse:  map[string]*layout.Layout{"layout.xml": lyt},
	}
	si := NewSourceIndex(&fakeSourceTree{}, &fakeSchemeTree{}, layouts, NewIgnoreList(nil, nil))

	graph := NewDependencyGraph()
	entries, matched := si.AddFile(context.Background(), graph, "unknown.txt", SourceTypeSource)
	if matched || entries != nil {
		t.Fatalf("expected no match, got %+v (matched=%v)", entries, matched)
	}
}

func TestIndex_AddFile_LayoutLookupError_ReturnsFalse(t *testing.T) {
	layouts := &fakeLayoutFinder{
		forDirErrs: map[string]error{".": errors.New("no layout")},
	}
	si := NewSourceIndex(&fakeSourceTree{}, &fakeSchemeTree{}, layouts, NewIgnoreList(nil, nil))

	graph := NewDependencyGraph()
	entries, matched := si.AddFile(context.Background(), graph, "about.md", SourceTypeSource)
	if matched || entries != nil {
		t.Fatalf("expected no match when layout lookup fails, got %+v (matched=%v)", entries, matched)
	}
}
