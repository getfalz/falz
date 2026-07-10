package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"testing"

	"github.com/valyala/fasttemplate"

	"craeft/internal/layout"
	"craeft/internal/pipeline"
)

// ---- fakes for the local interfaces declared in build.go ----

type fakeFileReader struct {
	files map[string][]byte
}

func (f *fakeFileReader) ReadFile(name string) ([]byte, error) {
	data, ok := f.files[name]
	if !ok {
		return nil, fmt.Errorf("no such file: %s", name)
	}
	return data, nil
}

type fakeOutputWriter struct {
	mu       sync.Mutex
	files    map[string][]byte
	removed  []string
	writeErr error
}

func (w *fakeOutputWriter) MkdirAll(path string, perm fs.FileMode) error { return nil }

func (w *fakeOutputWriter) WriteFile(name string, data []byte, perm fs.FileMode) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writeErr != nil {
		return w.writeErr
	}
	if w.files == nil {
		w.files = map[string][]byte{}
	}
	w.files[name] = append([]byte(nil), data...)
	return nil
}

func (w *fakeOutputWriter) Remove(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.removed = append(w.removed, name)
	return nil
}

func (w *fakeOutputWriter) written(name string) ([]byte, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	d, ok := w.files[name]
	return d, ok
}

type fakePipe struct {
	hash      string
	transform func(ctx context.Context, input []byte, vars pipeline.Vars) (pipeline.TransformResult, error)
}

func (p *fakePipe) Transform(ctx context.Context, input []byte, vars pipeline.Vars) (pipeline.TransformResult, error) {
	return p.transform(ctx, input, vars)
}
func (p *fakePipe) Config() pipeline.PipelineConfig { return pipeline.PipelineConfig{} }
func (p *fakePipe) Hash() string                    { return p.hash }

func uppercasePipe() *fakePipe {
	return &fakePipe{
		hash: "pipe-hash",
		transform: func(ctx context.Context, input []byte, vars pipeline.Vars) (pipeline.TransformResult, error) {
			return pipeline.TransformResult{Content: bytes.ToUpper(input)}, nil
		},
	}
}

type fakePipelineLoader struct {
	pipes map[string]pipeline.Pipe
}

func (f *fakePipelineLoader) Get(name string) (pipeline.Pipe, error) {
	if p, ok := f.pipes[name]; ok {
		return p, nil
	}
	return nil, pipeline.ErrPipelineNotFound
}

type fakeManifest struct {
	mu      sync.Mutex
	entries map[string]ManifestEntry
}

func (m *fakeManifest) Set(id string, entry ManifestEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = map[string]ManifestEntry{}
	}
	m.entries[id] = entry
}

func (m *fakeManifest) Get(id string) (ManifestEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	return e, ok
}

type fakeCache struct {
	mu           sync.Mutex
	has          map[string]bool
	storeCalls   []string
	restoreCalls []string
}

func (c *fakeCache) Has(key string) bool { return c.has[key] }

func (c *fakeCache) Store(key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.storeCalls = append(c.storeCalls, key)
	return nil
}

func (c *fakeCache) Restore(key string, outputPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreCalls = append(c.restoreCalls, outputPath)
	return nil
}

type fakeLayoutLoader struct {
	mu    sync.Mutex
	lyt   *layout.Layout
	entry *LayoutCacheEntry

	parseCalls int
	getCalls   int
}

func (f *fakeLayoutLoader) Parse(key LayoutKey) (*layout.Layout, error) {
	f.mu.Lock()
	f.parseCalls++
	f.mu.Unlock()
	return f.lyt, nil
}

func (f *fakeLayoutLoader) Get(ctx context.Context, key LayoutKey, contentVars map[string]string) (*LayoutCacheEntry, error) {
	f.mu.Lock()
	f.getCalls++
	f.mu.Unlock()
	return f.entry, nil
}

// ---- test helpers ----

// testLayout returns a real, parsed layout.Layout with one Content ("body",
// wrapped by an <html> template) and one top-level Asset ("style"), both
// outputting to "{FILENAME}.<ext>".
func testLayout(t *testing.T) *layout.Layout {
	t.Helper()
	xmlSrc := `<Layout tpl="&lt;html&gt;{{body}}&lt;/html&gt;">` +
		`<Content name="body" src="*.md" to="{FILENAME}.html"/>` +
		`<Asset name="style" src="*.css" to="{FILENAME}.css"/>` +
		`</Layout>`
	lyt, err := layout.Parse("site", []byte(xmlSrc))
	if err != nil {
		t.Fatalf("parse test layout: %v", err)
	}
	return lyt
}

// testLayoutEntry returns the LayoutCacheEntry counterpart of testLayout,
// i.e. what LayoutCache.Get would have produced after rendering fragments.
func testLayoutEntry(t *testing.T) *LayoutCacheEntry {
	t.Helper()
	tpl, err := fasttemplate.NewTemplate("<html>{{body}}</html>", "{{", "}}")
	if err != nil {
		t.Fatalf("parse test layout template: %v", err)
	}
	return &LayoutCacheEntry{
		layoutTpl: tpl,
		tags:      map[string]struct{}{"body": {}},
	}
}

func TestBuild_NoEntries(t *testing.T) {
	b := NewBuilder(&fakeFileReader{}, &fakeFileReader{}, &fakeOutputWriter{}, &fakeLayoutLoader{}, &fakePipelineLoader{}, &fakeManifest{}, nil, nil)

	stats, err := b.Build(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Scheduled != 0 || stats.Completed != 0 || stats.Written != 0 || len(stats.Errors) != 0 {
		t.Fatalf("expected zero stats, got %+v", stats)
	}
}

func TestBuild_ContentEntry_WritesRenderedOutput(t *testing.T) {
	src := &fakeFileReader{files: map[string][]byte{"about.md": []byte("hello")}}
	out := &fakeOutputWriter{}
	manifest := &fakeManifest{}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"text": uppercasePipe()}}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}

	b := NewBuilder(src, &fakeFileReader{}, out, layouts, pipes, manifest, nil, &BuilderConfig{Concurrency: 1})

	entry := &BuildEntry{
		ID: "about.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "about.md", TagName: "body", Pipeline: "text", BaseDir: ".",
	}

	stats, err := b.Build(context.Background(), []*BuildEntry{entry})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Written != 1 {
		t.Fatalf("expected 1 written, got %+v", stats)
	}

	data, ok := out.written("about.html")
	if !ok {
		t.Fatalf("expected output file about.html to be written; got files: %v", out.files)
	}
	if string(data) != "<html>HELLO</html>" {
		t.Fatalf("unexpected output: %q", data)
	}

	me, ok := manifest.Get(entry.ID)
	if !ok || me.OutputPath != "about.html" {
		t.Fatalf("expected manifest entry with output about.html, got %+v (ok=%v)", me, ok)
	}
}

// TestBuild_ContentEntry_PassesStructuredVarsToPipe is a regression test for
// a bug where the pipe transform received a flattened vars map: content vars
// merged at the top level, "vars" pointing at the global config variables
// (not the entry's own <Var> values), and "type"/"tag" as top-level keys.
// Pipes now receive "vars" (this entry's content vars), "globals" (config
// variables), and a nested "entry": {type, tag}.
func TestBuild_ContentEntry_PassesStructuredVarsToPipe(t *testing.T) {
	src := &fakeFileReader{files: map[string][]byte{"about.md": []byte("hello")}}
	out := &fakeOutputWriter{}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}

	var captured pipeline.Vars
	pipe := &fakePipe{hash: "h", transform: func(ctx context.Context, input []byte, vars pipeline.Vars) (pipeline.TransformResult, error) {
		captured = vars
		return pipeline.TransformResult{Content: input}, nil
	}}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"text": pipe}}

	b := NewBuilder(src, &fakeFileReader{}, out, layouts, pipes, &fakeManifest{}, nil, &BuilderConfig{
		Concurrency: 1,
		Variables:   map[string]any{"siteName": "Acme"},
	})

	entry := &BuildEntry{
		ID: "about.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "about.md", TagName: "body", Pipeline: "text", BaseDir: ".",
		ContentVars: map[string]string{"title": "Hello Page"},
	}

	if _, err := b.Build(context.Background(), []*BuildEntry{entry}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured == nil {
		t.Fatalf("expected pipe.Transform to be called")
	}

	gotVars, ok := captured["vars"].(map[string]string)
	if !ok || gotVars["title"] != "Hello Page" {
		t.Fatalf(`expected vars["vars"] to be the entry's ContentVars {"title":"Hello Page"}, got %#v`, captured["vars"])
	}

	gotGlobals, ok := captured["globals"].(map[string]any)
	if !ok || gotGlobals["siteName"] != "Acme" {
		t.Fatalf(`expected vars["globals"] to be the config Variables {"siteName":"Acme"}, got %#v`, captured["globals"])
	}

	gotEntry, ok := captured["entry"].(pipeline.Vars)
	if !ok || gotEntry["type"] != "content" || gotEntry["tag"] != "body" {
		t.Fatalf(`expected vars["entry"] to be {"type":"content","tag":"body"}, got %#v`, captured["entry"])
	}
}

func TestBuild_AssetEntry_SkipsLayoutTemplate(t *testing.T) {
	scheme := &fakeFileReader{files: map[string][]byte{"style.css": []byte("body{}")}}
	out := &fakeOutputWriter{}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"css": uppercasePipe()}}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}

	b := NewBuilder(&fakeFileReader{}, scheme, out, layouts, pipes, &fakeManifest{}, nil, &BuilderConfig{Concurrency: 1})

	entry := &BuildEntry{
		ID: "style.css:style", Type: AssetType, SourceRoot: SourceTypeScheme,
		SourcePath: "style.css", TagName: "style", Pipeline: "css", BaseDir: ".",
	}

	if _, err := b.Build(context.Background(), []*BuildEntry{entry}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := out.written("style.css")
	if !ok {
		t.Fatalf("expected style.css to be written; got %v", out.files)
	}
	if string(data) != "BODY{}" {
		t.Fatalf("expected raw pipe output without layout wrapping, got %q", data)
	}
}

func TestBuild_UnknownPipeline_SkipsEntryWithoutError(t *testing.T) {
	src := &fakeFileReader{files: map[string][]byte{"about.md": []byte("hello")}}
	out := &fakeOutputWriter{}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{}}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}

	b := NewBuilder(src, &fakeFileReader{}, out, layouts, pipes, &fakeManifest{}, nil, &BuilderConfig{Concurrency: 1})

	entry := &BuildEntry{
		ID: "about.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "about.md", TagName: "body", Pipeline: "missing", BaseDir: ".",
	}

	stats, err := b.Build(context.Background(), []*BuildEntry{entry})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats.Errors) != 0 {
		t.Fatalf("expected no errors for a skipped entry, got %v", stats.Errors)
	}
	if _, ok := out.written("about.html"); ok {
		t.Fatalf("expected nothing written for skipped entry")
	}
}

// TestBuild_SkippedEntryDoesNotBlockOtherEntries guards against a worker
// abandoning the whole queue when it encounters a skipped entry.
func TestBuild_SkippedEntryDoesNotBlockOtherEntries(t *testing.T) {
	src := &fakeFileReader{files: map[string][]byte{
		"skip.md":  []byte("skip"),
		"about.md": []byte("hello"),
	}}
	out := &fakeOutputWriter{}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"text": uppercasePipe()}}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}

	// Concurrency 1 so a single worker processes both entries sequentially.
	b := NewBuilder(src, &fakeFileReader{}, out, layouts, pipes, &fakeManifest{}, nil, &BuilderConfig{Concurrency: 1})

	skipped := &BuildEntry{
		ID: "skip.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "skip.md", TagName: "body", Pipeline: "missing", BaseDir: ".",
	}
	normal := &BuildEntry{
		ID: "about.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "about.md", TagName: "body", Pipeline: "text", BaseDir: ".",
	}

	if _, err := b.Build(context.Background(), []*BuildEntry{skipped, normal}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := out.written("about.html"); !ok {
		t.Fatalf("entry queued after a skipped one was never built (worker exited early on skip)")
	}
}

func TestBuild_ReadFileError_IsReturned(t *testing.T) {
	out := &fakeOutputWriter{}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"text": uppercasePipe()}}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}

	b := NewBuilder(&fakeFileReader{}, &fakeFileReader{}, out, layouts, pipes, &fakeManifest{}, nil, &BuilderConfig{Concurrency: 1})

	entry := &BuildEntry{
		ID: "missing.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "missing.md", TagName: "body", Pipeline: "text", BaseDir: ".",
	}

	stats, err := b.Build(context.Background(), []*BuildEntry{entry})
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(stats.Errors) != 1 {
		t.Fatalf("expected 1 error in stats, got %v", stats.Errors)
	}
}

func TestBuild_TransformError_SkipsWrite(t *testing.T) {
	src := &fakeFileReader{files: map[string][]byte{"about.md": []byte("hello")}}
	out := &fakeOutputWriter{}
	failErr := errors.New("boom")
	pipe := &fakePipe{hash: "h", transform: func(ctx context.Context, input []byte, vars pipeline.Vars) (pipeline.TransformResult, error) {
		return pipeline.TransformResult{}, failErr
	}}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"text": pipe}}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}

	b := NewBuilder(src, &fakeFileReader{}, out, layouts, pipes, &fakeManifest{}, nil, &BuilderConfig{Concurrency: 1})

	entry := &BuildEntry{
		ID: "about.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "about.md", TagName: "body", Pipeline: "text", BaseDir: ".",
	}

	_, err := b.Build(context.Background(), []*BuildEntry{entry})
	if err == nil || !errors.Is(err, failErr) {
		t.Fatalf("expected wrapped transform error, got %v", err)
	}
	if _, ok := out.written("about.html"); ok {
		t.Fatalf("expected no output written when transform fails")
	}
}

func TestBuild_CacheHit_RestoresWithoutTransform(t *testing.T) {
	transformCalled := false
	pipe := &fakePipe{hash: "h", transform: func(ctx context.Context, input []byte, vars pipeline.Vars) (pipeline.TransformResult, error) {
		transformCalled = true
		return pipeline.TransformResult{Content: input}, nil
	}}
	src := &fakeFileReader{files: map[string][]byte{"about.md": []byte("hello")}}
	out := &fakeOutputWriter{}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"text": pipe}}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}
	manifest := &fakeManifest{entries: map[string]ManifestEntry{
		"about.md:body": {Hash: "hash-1", OutputPath: "about.html"},
	}}
	cache := &fakeCache{has: map[string]bool{"hash-1": true}}

	b := NewBuilder(src, &fakeFileReader{}, out, layouts, pipes, manifest, cache, &BuilderConfig{Concurrency: 1, UseCAS: true})

	entry := &BuildEntry{
		ID: "about.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "about.md", TagName: "body", Pipeline: "text", BaseDir: ".", Hash: "hash-1",
	}

	stats, err := b.Build(context.Background(), []*BuildEntry{entry})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transformCalled {
		t.Fatalf("expected pipe.Transform not to be called on a cache hit")
	}
	if stats.CacheHits != 1 {
		t.Fatalf("expected 1 cache hit, got %+v", stats)
	}
	if len(cache.restoreCalls) != 1 || cache.restoreCalls[0] != "about.html" {
		t.Fatalf("expected CAS restore to about.html, got %v", cache.restoreCalls)
	}
}

func TestBuild_CacheMiss_StoresThenRestoresFromCAS(t *testing.T) {
	src := &fakeFileReader{files: map[string][]byte{"about.md": []byte("hello")}}
	out := &fakeOutputWriter{}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"text": uppercasePipe()}}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}
	cache := &fakeCache{has: map[string]bool{}}

	b := NewBuilder(src, &fakeFileReader{}, out, layouts, pipes, &fakeManifest{}, cache, &BuilderConfig{Concurrency: 1, UseCAS: true})

	entry := &BuildEntry{
		ID: "about.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "about.md", TagName: "body", Pipeline: "text", BaseDir: ".", Hash: "hash-1",
	}

	if _, err := b.Build(context.Background(), []*BuildEntry{entry}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cache.storeCalls) != 1 || cache.storeCalls[0] != "hash-1" {
		t.Fatalf("expected CAS store for hash-1, got %v", cache.storeCalls)
	}
	if len(cache.restoreCalls) != 1 || cache.restoreCalls[0] != "about.html" {
		t.Fatalf("expected CAS restore to about.html, got %v", cache.restoreCalls)
	}
	if _, ok := out.written("about.html"); ok {
		t.Fatalf("expected write to go through CAS restore, not a direct WriteFile")
	}
}

func TestBuild_RemovesStaleOutputWhenOutputPathChanges(t *testing.T) {
	src := &fakeFileReader{files: map[string][]byte{"about.md": []byte("hello")}}
	out := &fakeOutputWriter{}
	pipes := &fakePipelineLoader{pipes: map[string]pipeline.Pipe{"text": uppercasePipe()}}
	layouts := &fakeLayoutLoader{lyt: testLayout(t), entry: testLayoutEntry(t)}
	manifest := &fakeManifest{entries: map[string]ManifestEntry{
		"about.md:body": {Hash: "old-hash", OutputPath: "old/about.html"},
	}}

	b := NewBuilder(src, &fakeFileReader{}, out, layouts, pipes, manifest, nil, &BuilderConfig{Concurrency: 1})

	entry := &BuildEntry{
		ID: "about.md:body", Type: ContentType, SourceRoot: SourceTypeSource,
		SourcePath: "about.md", TagName: "body", Pipeline: "text", BaseDir: ".", Hash: "new-hash",
	}

	if _, err := b.Build(context.Background(), []*BuildEntry{entry}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, r := range out.removed {
		if r == "old/about.html" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected stale output old/about.html to be removed, got %v", out.removed)
	}
}
