package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// newTestProjectConfig sets up a minimal, valid project on disk: an empty
// pipes dir, a scheme dir with a single layout wrapping "body" content in
// <html>...</html>, and an empty source/output dir. Callers add source
// files as needed.
func newTestProjectConfig(t *testing.T) *CraeftConfig {
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

	return &CraeftConfig{
		OutputPath: outputPath,
		SourcePath: sourcePath,
		SchemePath: schemePath,
		PipesPath:  pipesPath,
		Layout:     "layout.xml",
		Build: BuildConfig{
			Concurrency: 1,
		},
	}
}

func writeSourceFile(t *testing.T, cfg *CraeftConfig, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cfg.SourcePath, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write source file %s: %v", name, err)
	}
}

func TestProject_New_MissingSchemeDir_ReturnsError(t *testing.T) {
	cfg := newTestProjectConfig(t)
	cfg.SchemePath = filepath.Join(t.TempDir(), "does-not-exist")

	if _, err := New(cfg); err == nil {
		t.Fatalf("expected New to fail when the scheme dir doesn't exist")
	}
}

func TestProject_Build_WritesOutputAndManifest(t *testing.T) {
	cfg := newTestProjectConfig(t)
	writeSourceFile(t, cfg, "about.md", "hello world")

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data, err := p.Output.ReadFile("about.html")
	if err != nil {
		t.Fatalf("expected about.html to exist: %v", err)
	}
	if string(data) != "<html>hello world</html>" {
		t.Fatalf("unexpected output: %q", data)
	}

	if _, ok := p.Manifest.Get("about.md:body"); !ok {
		t.Fatalf("expected manifest entry for about.md:body")
	}
	if _, err := p.Output.ReadFile(manifestoFile); err != nil {
		t.Fatalf("expected manifest to be persisted to disk: %v", err)
	}
}

func TestProject_Build_SecondRunWithNoChanges_LeavesOutputIntact(t *testing.T) {
	cfg := newTestProjectConfig(t)
	writeSourceFile(t, cfg, "about.md", "hello world")

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build (1st): %v", err)
	}
	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build (2nd): %v", err)
	}

	data, err := p.Output.ReadFile("about.html")
	if err != nil {
		t.Fatalf("expected about.html to still exist: %v", err)
	}
	if string(data) != "<html>hello world</html>" {
		t.Fatalf("unexpected output after no-op rebuild: %q", data)
	}
}

func TestProject_Build_PicksUpChangedSourceOnRebuild(t *testing.T) {
	cfg := newTestProjectConfig(t)
	writeSourceFile(t, cfg, "about.md", "hello world")

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build (1st): %v", err)
	}

	writeSourceFile(t, cfg, "about.md", "updated content")
	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build (2nd): %v", err)
	}

	data, err := p.Output.ReadFile("about.html")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "<html>updated content</html>" {
		t.Fatalf("expected rebuild to pick up the change, got %q", data)
	}
}

func TestProject_ForceBuild_RebuildsSingleEntryByID(t *testing.T) {
	cfg := newTestProjectConfig(t)
	writeSourceFile(t, cfg, "about.md", "hello world")

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build: %v", err)
	}

	stats, err := p.ForceBuild(context.Background(), "about.md:body")
	if err != nil {
		t.Fatalf("ForceBuild: %v", err)
	}
	if stats.Written != 1 {
		t.Fatalf("expected exactly 1 written entry, got %+v", stats)
	}
}

func TestProject_ForceBuild_UnknownEntry_ReturnsError(t *testing.T) {
	cfg := newTestProjectConfig(t)
	writeSourceFile(t, cfg, "about.md", "hello world")

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if _, err := p.ForceBuild(context.Background(), "no-such-entry"); err == nil {
		t.Fatalf("expected ForceBuild to fail for an unknown entry ID")
	}
}

func TestProject_CleanCache_RemovesManifestAndOutputs(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	cfg := newTestProjectConfig(t)
	writeSourceFile(t, cfg, "about.md", "hello world")

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := p.Build(context.Background()); err != nil {
		t.Fatalf("Build: %v", err)
	}
	p.Close()

	if err := CleanCache(cfg); err != nil {
		t.Fatalf("CleanCache: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.OutputPath, "about.html")); err == nil {
		t.Fatalf("expected about.html to be removed by CleanCache")
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputPath, manifestoFile)); err == nil {
		t.Fatalf("expected manifest file to be removed by CleanCache")
	}
}
