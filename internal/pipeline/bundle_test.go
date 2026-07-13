package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"craeft/internal/utils"
)

// newTestBundleDirs writes files (keyed by path relative to the pipe's own
// directory) under pipesPath/<pipeName>/ and returns RootFS handles for the
// pipe directory and its pipes-root parent, as newBundle expects.
func newTestBundleDirs(t *testing.T, pipeName string, files map[string]string) (pipeRoot, pipesRoot *utils.RootFS) {
	t.Helper()
	base := t.TempDir()
	pipesPath := filepath.Join(base, "pipes")
	pipeDir := filepath.Join(pipesPath, pipeName)

	for name, content := range files {
		full := filepath.Join(pipeDir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	pr, err := utils.OpenRootFs(pipeDir)
	if err != nil {
		t.Fatalf("open pipe root: %v", err)
	}
	t.Cleanup(func() { _ = pr.Close() })

	prt, err := utils.OpenRootFs(pipesPath)
	if err != nil {
		t.Fatalf("open pipes root: %v", err)
	}
	t.Cleanup(func() { _ = prt.Close() })

	return pr, prt
}

func newTestBundle(t *testing.T, pipeRoot, pipesRoot *utils.RootFS, entry string) *bundle {
	t.Helper()
	b, err := newBundle(entry, pipeRoot, pipesRoot, false)
	if err != nil {
		t.Fatalf("newBundle: %v", err)
	}
	t.Cleanup(b.close)
	return b
}

func TestBundle_Build_BundlesEntryPoint(t *testing.T) {
	pipeRoot, pipesRoot := newTestBundleDirs(t, "mypipe", map[string]string{
		"index.ts": `export function transform(input: any) { return "ok"; }`,
	})
	b := newTestBundle(t, pipeRoot, pipesRoot, "index.ts")

	out, err := b.build(false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(out, "__pipe__") {
		t.Fatalf("expected the bundle to define the __pipe__ global, got:\n%s", out)
	}
}

func TestBundle_Build_LocalRelativeImportWithinPipeDir_Succeeds(t *testing.T) {
	pipeRoot, pipesRoot := newTestBundleDirs(t, "mypipe", map[string]string{
		"index.ts":  `import { greeting } from "./helper"; export function transform(input: any) { return greeting; }`,
		"helper.ts": `export const greeting = "hi";`,
	})
	b := newTestBundle(t, pipeRoot, pipesRoot, "index.ts")

	out, err := b.build(false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("expected the imported helper's content in the bundle, got:\n%s", out)
	}
}

func TestBundle_Build_Debug_WritesDbgFileIntoPipeFolder(t *testing.T) {
	pipeRoot, pipesRoot := newTestBundleDirs(t, "mypipe", map[string]string{
		"index.ts": `export function transform(input: any) { return "ok"; }`,
	})
	b := newTestBundle(t, pipeRoot, pipesRoot, "index.ts")

	if _, err := b.build(true); err != nil {
		t.Fatalf("build: %v", err)
	}

	data, err := pipeRoot.ReadFile("pipeline.dbg.js")
	if err != nil {
		t.Fatalf("expected pipeline.dbg.js in the pipe's own folder: %v", err)
	}
	if !strings.Contains(string(data), "__pipe__") {
		t.Fatalf("expected the debug file to contain the bundled output, got:\n%s", data)
	}
}

func TestBundle_Build_SyntaxError_ReturnsError(t *testing.T) {
	pipeRoot, pipesRoot := newTestBundleDirs(t, "mypipe", map[string]string{
		"index.ts": `export function transform( { this is not valid TS`,
	})
	b := newTestBundle(t, pipeRoot, pipesRoot, "index.ts")

	if _, err := b.build(false); err == nil {
		t.Fatalf("expected an error for invalid TypeScript syntax")
	}
}

// An import that resolves to a file outside the pipes root entirely must be
// rejected by the sandbox plugin, even though esbuild itself would happily
// resolve it.
func TestBundle_Build_ImportEscapingPipesRoot_IsRejected(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "outside.ts"), []byte(`export const secret = "leaked";`), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	pipesPath := filepath.Join(base, "pipes")
	pipeDir := filepath.Join(pipesPath, "mypipe")
	if err := os.MkdirAll(pipeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	indexSrc := `import { secret } from "../../outside"; export function transform(input: any) { return secret; }`
	if err := os.WriteFile(filepath.Join(pipeDir, "index.ts"), []byte(indexSrc), 0o644); err != nil {
		t.Fatalf("write index.ts: %v", err)
	}

	pipeRoot, err := utils.OpenRootFs(pipeDir)
	if err != nil {
		t.Fatalf("open pipe root: %v", err)
	}
	defer pipeRoot.Close()
	pipesRoot, err := utils.OpenRootFs(pipesPath)
	if err != nil {
		t.Fatalf("open pipes root: %v", err)
	}
	defer pipesRoot.Close()

	b := newTestBundle(t, pipeRoot, pipesRoot, "index.ts")

	_, err = b.build(false)
	if err == nil {
		t.Fatalf("expected the sandbox to reject an import escaping the pipes root")
	}
	if !strings.Contains(err.Error(), "escapes pipes root") {
		t.Fatalf("expected a sandbox-escape error, got: %v", err)
	}
}
