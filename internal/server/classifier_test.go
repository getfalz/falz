package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"craeft/internal/pipeline"
	"craeft/internal/project"
	"craeft/internal/utils"
)

func newTestSchemeRoot(t *testing.T) *utils.RootFS {
	t.Helper()
	root, err := utils.NewRootFs(t.TempDir())
	if err != nil {
		t.Fatalf("new root fs: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func newTestPipes(t *testing.T) *pipeline.Registry {
	t.Helper()
	reg, err := pipeline.NewRegistry(&pipeline.RegistryConfig{PipesPath: t.TempDir()})
	if err != nil {
		t.Fatalf("new pipes registry: %v", err)
	}
	return reg
}

func writeSchemeFile(t *testing.T, root *utils.RootFS, relPath, content string) {
	t.Helper()
	full := filepath.Join(root.Path(), relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func newTestClassifier(t *testing.T, root *utils.RootFS) *FileClassifier {
	t.Helper()
	layouts := project.NewLayoutCache(root, newTestPipes(t), "layout.xml")
	proj := &project.Project{
		Config:  &project.CraeftConfig{Layout: "layout.xml"},
		Layouts: layouts,
	}
	return NewFileClassifier(proj, "", root.Path(), "", "", project.NewIgnoreList(nil, nil))
}

// TestClassifySchemeFile_AssetNestedUnderRootLayout_UsesLayoutDirAsMatchBase
// is a regression test for a bug where classifySchemeFile matched a scheme
// file's Asset/Template src pattern against the *changed file's own*
// directory instead of the *governing layout's* directory. This broke as
// soon as an asset (or template) lived in a subdirectory relative to the
// layout that declares it and the layout itself was found by walking up
// from that subdirectory (e.g. assets under "css/" declared by a root-level
// layout.xml): the match base and the pattern's implied base diverged, so
// the file was never classified.
func TestClassifySchemeFile_AssetNestedUnderRootLayout_UsesLayoutDirAsMatchBase(t *testing.T) {
	root := newTestSchemeRoot(t)
	writeSchemeFile(t, root, "layout.xml",
		`<Layout tpl="x"><Asset name="style" src="css/*.css" to="css/{FILENAME}.css"/></Layout>`)
	writeSchemeFile(t, root, "css/style.css", "body{}")

	fc := newTestClassifier(t, root)

	got, err := fc.classifySchemeFile(context.Background(), "css/style.css")
	if err != nil {
		t.Fatalf("classifySchemeFile: %v", err)
	}
	if got != FileTypeSchemeAsset {
		t.Fatalf("expected FileTypeSchemeAsset, got %q", got)
	}
}

// TestClassifySchemeFile_LayoutTemplateFile_IsClassified is a regression
// test for a bug where a layout's own template file (Layout's `src`/TplPath
// attribute) was never matched against any file type at all, so edits to it
// went unclassified (FileTypeUnknown) and were silently ignored by the
// watcher/hot-reload pipeline.
func TestClassifySchemeFile_LayoutTemplateFile_IsClassified(t *testing.T) {
	root := newTestSchemeRoot(t)
	writeSchemeFile(t, root, "layout.xml", `<Layout src="tpl.html"/>`)
	writeSchemeFile(t, root, "tpl.html", "<html></html>")

	fc := newTestClassifier(t, root)

	got, err := fc.classifySchemeFile(context.Background(), "tpl.html")
	if err != nil {
		t.Fatalf("classifySchemeFile: %v", err)
	}
	if got != FileTypeLayoutTemplate {
		t.Fatalf("expected FileTypeLayoutTemplate, got %q", got)
	}
}

func TestClassifySchemeFile_LayoutFile_IsClassifiedAsLayout(t *testing.T) {
	root := newTestSchemeRoot(t)
	writeSchemeFile(t, root, "layout.xml", `<Layout tpl="x"/>`)

	fc := newTestClassifier(t, root)

	got, err := fc.classifySchemeFile(context.Background(), "layout.xml")
	if err != nil {
		t.Fatalf("classifySchemeFile: %v", err)
	}
	if got != FileTypeLayout {
		t.Fatalf("expected FileTypeLayout, got %q", got)
	}
}

func TestClassifySchemeFile_FragmentFile_IsClassifiedAsFragment(t *testing.T) {
	root := newTestSchemeRoot(t)
	writeSchemeFile(t, root, "layout.xml",
		`<Layout tpl="x"><Fragment name="frag" src="frag.html" pipe=""/></Layout>`)
	writeSchemeFile(t, root, "frag.html", "FRAG")

	fc := newTestClassifier(t, root)

	got, err := fc.classifySchemeFile(context.Background(), "frag.html")
	if err != nil {
		t.Fatalf("classifySchemeFile: %v", err)
	}
	if got != FileTypeFragment {
		t.Fatalf("expected FileTypeFragment, got %q", got)
	}
}

func TestClassifySchemeFile_UnrelatedFile_IsUnknown(t *testing.T) {
	root := newTestSchemeRoot(t)
	writeSchemeFile(t, root, "layout.xml", `<Layout tpl="x"/>`)
	writeSchemeFile(t, root, "notes.txt", "just notes")

	fc := newTestClassifier(t, root)

	got, err := fc.classifySchemeFile(context.Background(), "notes.txt")
	if err != nil {
		t.Fatalf("classifySchemeFile: %v", err)
	}
	if got != FileTypeUnknown {
		t.Fatalf("expected FileTypeUnknown, got %q", got)
	}
}

// TestClassify_SourceContentFile_ClassifiedAsContent is a real end-to-end
// usage test of the watcher's entrypoint: a raw filesystem event for a
// content file under the project's source root should resolve to a
// FileTypeContent change with the path relative to that root.
func TestClassify_SourceContentFile_ClassifiedAsContent(t *testing.T) {
	p, cfg := newTestServerProject(t)
	writeServerSourceFile(t, cfg, "about.md", "hello world")

	fc := NewFileClassifier(p, cfg.SourcePath, cfg.SchemePath, cfg.PipesPath, cfg.OutputPath, p.Ignore)

	got := fc.Classify(context.Background(), RawChange{
		Path: filepath.Join(cfg.SourcePath, "about.md"),
		Mod:  ModTypeModified,
		Time: time.Now(),
	})
	if got == nil {
		t.Fatalf("expected a classification, got nil")
	}
	if got.Type != FileTypeContent || got.Path != "about.md" || got.Mod != ModTypeModified {
		t.Fatalf("unexpected classification: %+v", got)
	}
}

// TestClassify_IgnoredSourceFile_ReturnsNil ensures files matched by the
// project's ignore list (e.g. dotfiles, editor swap files configured via
// `ignore:` in config.yaml) never reach a handler.
func TestClassify_IgnoredSourceFile_ReturnsNil(t *testing.T) {
	p, cfg := newTestServerProject(t)
	p.Ignore.Reload([]string{"*.swp"})
	writeServerSourceFile(t, cfg, "about.md.swp", "swap file junk")

	fc := NewFileClassifier(p, cfg.SourcePath, cfg.SchemePath, cfg.PipesPath, cfg.OutputPath, p.Ignore)

	got := fc.Classify(context.Background(), RawChange{
		Path: filepath.Join(cfg.SourcePath, "about.md.swp"),
		Mod:  ModTypeCreated,
		Time: time.Now(),
	})
	if got != nil {
		t.Fatalf("expected an ignored file to classify to nil, got %+v", got)
	}
}

// TestClassify_UnrelatedAbsolutePath_ReturnsNil covers an event for a path
// outside every configured root (source/scheme/pipes/config) -- e.g. a
// sibling directory the OS watcher happened to report -- which must be
// silently dropped rather than misattributed to one of the roots.
func TestClassify_UnrelatedAbsolutePath_ReturnsNil(t *testing.T) {
	p, cfg := newTestServerProject(t)
	fc := NewFileClassifier(p, cfg.SourcePath, cfg.SchemePath, cfg.PipesPath, cfg.OutputPath, p.Ignore)

	got := fc.Classify(context.Background(), RawChange{
		Path: filepath.Join(t.TempDir(), "unrelated.txt"),
		Mod:  ModTypeModified,
		Time: time.Now(),
	})
	if got != nil {
		t.Fatalf("expected an unrelated path to classify to nil, got %+v", got)
	}
}
