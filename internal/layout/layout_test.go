package layout

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"craeft/internal/pipeline"
	"craeft/internal/utils"
)

func newTestRoot(t *testing.T) *utils.RootFS {
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

func writeFile(t *testing.T, root *utils.RootFS, relPath, content string) {
	t.Helper()
	full := filepath.Join(root.Path(), relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

// TestFragment_Render_UnknownPipe_ReturnsErrorWithoutPanic is a regression
// test for a nil-pointer bug: when rctx.Pipelines.Get(f.Pipe) failed, `pipe`
// was nil, but the error path used to call pipe.Config().PipelineName on it
// anyway. It was fixed to report f.Pipe (the string) instead. This test
// exercises the "pipe not found" branch directly, which is exactly the path
// that used to panic.
func TestFragment_Render_UnknownPipe_ReturnsErrorWithoutPanic(t *testing.T) {
	root := newTestRoot(t)
	writeFile(t, root, "frag.html", "FRAG")

	lyt := parseTestLayout(t, ".", `<Layout tpl="x"><Fragment name="frag" src="frag.html" pipe="does-not-exist"/></Layout>`)
	frag := lyt.Fragments()[0]

	rctx := RenderCtx{Pipelines: newTestPipes(t), FS: root}

	_, _, err := frag.Render(context.Background(), rctx, nil)
	if err == nil {
		t.Fatalf("expected an error for an unknown pipe")
	}
	if got := err.Error(); !strings.Contains(got, "does-not-exist") {
		t.Fatalf("expected error to mention the missing pipe name %q, got %q", "does-not-exist", got)
	}
}

func parseTestLayout(t *testing.T, path, xmlSrc string) *Layout {
	t.Helper()
	lyt, err := Parse(path, []byte(xmlSrc))
	if err != nil {
		t.Fatalf("parse layout: %v", err)
	}
	return lyt
}

func TestLayout_Validate_RequiresTplOrTplPath(t *testing.T) {
	if _, err := Parse(".", []byte(`<Layout><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`)); err == nil {
		t.Fatalf("expected an error when neither tpl nor src is set")
	}
}

func TestLayout_Parse_ContentMissingRequiredFields(t *testing.T) {
	if _, err := Parse(".", []byte(`<Layout tpl="x"><Content name="body"/></Layout>`)); err == nil {
		t.Fatalf("expected an error for a Content missing 'src' and 'to'")
	}
}

func TestLayout_Render_InlinesFragmentAndPreservesContentPlaceholder(t *testing.T) {
	root := newTestRoot(t)
	writeFile(t, root, "frag.html", "FRAGTXT")

	lyt := parseTestLayout(t, ".", `<Layout tpl="&lt;html&gt;{{frag}}{{body}}&lt;/html&gt;">`+
		`<Content name="body" src="*.md" to="{FILENAME}.html"/>`+
		`<Fragment name="frag" src="frag.html" pipe=""/>`+
		`</Layout>`)

	rctx := RenderCtx{Pipelines: newTestPipes(t), FS: root}
	rendered, deps, err := lyt.Render(context.Background(), rctx, ".", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := "<html>FRAGTXT{{body}}</html>"
	if string(rendered) != want {
		t.Fatalf("got %q, want %q", rendered, want)
	}
	if len(deps) != 1 || deps[0] != "frag.html" {
		t.Fatalf("expected deps to include frag.html, got %+v", deps)
	}
}

func TestLayout_Render_MissingFragmentFile_ReturnsError(t *testing.T) {
	root := newTestRoot(t)

	lyt := parseTestLayout(t, ".", `<Layout tpl="{{frag}}"><Fragment name="frag" src="missing.html" pipe=""/></Layout>`)

	rctx := RenderCtx{Pipelines: newTestPipes(t), FS: root}
	if _, _, err := lyt.Render(context.Background(), rctx, ".", nil); err == nil {
		t.Fatalf("expected an error when the fragment file doesn't exist")
	}
}

func TestLayout_ContentOutput_SubstitutesFilenameAndExt(t *testing.T) {
	lyt := parseTestLayout(t, ".", `<Layout tpl="x"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`)

	out, err := lyt.ContentOutput("body", OutputEntry{SourcePath: "about.md", BaseDir: "."})
	if err != nil {
		t.Fatalf("ContentOutput: %v", err)
	}
	if out != "about.html" {
		t.Fatalf("got %q, want %q", out, "about.html")
	}
}

func TestLayout_ContentOutput_SubstitutesAliasAndHash(t *testing.T) {
	lyt := parseTestLayout(t, ".", `<Layout tpl="x"><Content name="body" src="*.md" to="{ALIAS}-{HASH:6}.html"/></Layout>`)

	out, err := lyt.ContentOutput("body", OutputEntry{
		SourcePath: "about.md", BaseDir: ".", Alias: "custom-name", Hash: "abcdef1234567890",
	})
	if err != nil {
		t.Fatalf("ContentOutput: %v", err)
	}
	if out != "custom-name-abcdef.html" {
		t.Fatalf("got %q, want %q", out, "custom-name-abcdef.html")
	}
}

func TestLayout_ContentOutput_UnknownContent_ReturnsError(t *testing.T) {
	lyt := parseTestLayout(t, ".", `<Layout tpl="x"><Content name="body" src="*.md" to="{FILENAME}.html"/></Layout>`)

	if _, err := lyt.ContentOutput("does-not-exist", OutputEntry{SourcePath: "about.md"}); err == nil {
		t.Fatalf("expected an error for an unknown content name")
	}
}

func TestLayout_CollectDeps_ReturnsTemplateAndFragmentPaths(t *testing.T) {
	root := newTestRoot(t)
	writeFile(t, root, "layout.tpl.html", "<html>{{frag}}</html>")
	writeFile(t, root, "frag.html", "FRAG")

	lyt := parseTestLayout(t, ".", `<Layout src="layout.tpl.html"><Fragment name="frag" src="frag.html" pipe=""/></Layout>`)

	rctx := RenderCtx{Pipelines: newTestPipes(t), FS: root}
	deps, err := lyt.CollectDeps(context.Background(), rctx, ".")
	if err != nil {
		t.Fatalf("CollectDeps: %v", err)
	}

	found := map[string]bool{}
	for _, d := range deps {
		found[d] = true
	}
	if !found["layout.tpl.html"] || !found["frag.html"] {
		t.Fatalf("expected deps to include both the template and the fragment, got %+v", deps)
	}
}
