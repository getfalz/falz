package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"craeft/internal/utils"
)

// newTestRegistryConfig lays out a minimal pipes directory on disk with a
// single JS pipe (name/entry point given) and returns a ready-to-use
// RegistryConfig.
func newTestRegistryConfig(t *testing.T, pipeName, indexTS string) *RegistryConfig {
	t.Helper()
	base := t.TempDir()
	pipesPath := filepath.Join(base, "pipes")
	pipeDir := filepath.Join(pipesPath, pipeName)
	if err := os.MkdirAll(pipeDir, 0o755); err != nil {
		t.Fatalf("mkdir pipe dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pipeDir, "config.yaml"), []byte("name: "+pipeName+"\nentryPoint: index.ts\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pipeDir, "index.ts"), []byte(indexTS), 0o644); err != nil {
		t.Fatalf("write index.ts: %v", err)
	}

	sourceRoot, err := utils.NewRootFs(filepath.Join(base, "src"))
	if err != nil {
		t.Fatalf("new source root: %v", err)
	}
	t.Cleanup(func() { _ = sourceRoot.Close() })

	return &RegistryConfig{Pipes: []string{pipeName}, PipesPath: pipesPath, SourceRoot: sourceRoot}
}

func newTestRegistry(t *testing.T, cfg *RegistryConfig) *Registry {
	t.Helper()
	r, err := NewRegistry(cfg)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestRegistry_Get_BuiltinsAvailableWithoutLoad(t *testing.T) {
	r := newTestRegistry(t, &RegistryConfig{PipesPath: t.TempDir()})

	for _, name := range []string{"md", "tpl", ""} {
		if _, err := r.Get(name); err != nil {
			t.Errorf("Get(%q): %v", name, err)
		}
	}
}

func TestRegistry_Get_UnknownPipe_ReturnsErrPipelineNotFound(t *testing.T) {
	r := newTestRegistry(t, &RegistryConfig{PipesPath: t.TempDir()})

	_, err := r.Get("does-not-exist")
	if !errors.Is(err, ErrPipelineNotFound) {
		t.Fatalf("expected ErrPipelineNotFound, got %v", err)
	}
}

func TestRegistry_Load_JSPipe_ThenGet_TransformsInput(t *testing.T) {
	cfg := newTestRegistryConfig(t, "myPipe", `export function transform(input) { return "custom:" + input.toString(); }`)
	r := newTestRegistry(t, cfg)

	if err := r.Load("myPipe"); err != nil {
		t.Fatalf("Load: %v", err)
	}

	pipe, err := r.Get("myPipe")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	result, err := pipe.Transform(context.Background(), []byte("hi"), nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if string(result.Content) != "custom:hi" {
		t.Fatalf("got %q, want %q", result.Content, "custom:hi")
	}
}

func TestRegistry_Load_NonExistentDirectory_NonBuiltinName_ReturnsError(t *testing.T) {
	r := newTestRegistry(t, &RegistryConfig{PipesPath: t.TempDir()})

	if err := r.Load("does-not-exist"); err == nil {
		t.Fatalf("expected an error loading a nonexistent, non-builtin pipe directory")
	}
}

func TestRegistry_Load_NonExistentDirectory_BuiltinName_IsNoop(t *testing.T) {
	r := newTestRegistry(t, &RegistryConfig{PipesPath: t.TempDir()})

	if err := r.Load("md"); err != nil {
		t.Fatalf("expected loading a builtin name with no on-disk directory to be a no-op, got %v", err)
	}
	if _, err := r.Get("md"); err != nil {
		t.Fatalf("Get(md): %v", err)
	}
}

func TestRegistry_Load_ReservedSharedName_IsSkippedWithoutError(t *testing.T) {
	r := newTestRegistry(t, &RegistryConfig{PipesPath: t.TempDir()})

	if err := r.Load(SharedPipeFolder); err != nil {
		t.Fatalf("expected loading the reserved %q name to be skipped without error, got %v", SharedPipeFolder, err)
	}
}

func TestRegistry_LoadAll_AggregatesErrorsAndLoadsTheRest(t *testing.T) {
	cfg := newTestRegistryConfig(t, "goodPipe", `export function transform(input) { return "ok"; }`)
	cfg.Pipes = []string{"goodPipe", "missingPipe"}
	r := newTestRegistry(t, cfg)

	err := r.LoadAll()
	if err == nil {
		t.Fatalf("expected LoadAll to report the missing pipe's error")
	}
	if _, getErr := r.Get("goodPipe"); getErr != nil {
		t.Fatalf("expected goodPipe to still be loaded despite missingPipe failing: %v", getErr)
	}
}

func TestRegistry_Reload_UnloadedPipe_ReturnsError(t *testing.T) {
	r := newTestRegistry(t, &RegistryConfig{PipesPath: t.TempDir()})

	if err := r.Reload("never-loaded"); err == nil {
		t.Fatalf("expected an error reloading a pipe that was never loaded")
	}
}

func TestRegistry_Reload_PicksUpSourceChanges(t *testing.T) {
	cfg := newTestRegistryConfig(t, "myPipe", `export function transform(input) { return "v1"; }`)
	r := newTestRegistry(t, cfg)

	if err := r.Load("myPipe"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	pipe, err := r.Get("myPipe")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	before, err := pipe.Transform(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Transform (before): %v", err)
	}
	if string(before.Content) != "v1" {
		t.Fatalf("got %q, want %q", before.Content, "v1")
	}

	indexPath := filepath.Join(cfg.PipesPath, "myPipe", "index.ts")
	if err := os.WriteFile(indexPath, []byte(`export function transform(input) { return "v2"; }`), 0o644); err != nil {
		t.Fatalf("update index.ts: %v", err)
	}
	if err := r.Reload("myPipe"); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	after, err := pipe.Transform(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Transform (after): %v", err)
	}
	if string(after.Content) != "v2" {
		t.Fatalf("got %q, want %q", after.Content, "v2")
	}
}

func TestRegistry_UnloadAll_RemovesJSPipesButKeepsBuiltins(t *testing.T) {
	cfg := newTestRegistryConfig(t, "myPipe", `export function transform(input) { return "ok"; }`)
	r := newTestRegistry(t, cfg)

	if err := r.Load("myPipe"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	r.UnloadAll()

	if _, err := r.Get("myPipe"); !errors.Is(err, ErrPipelineNotFound) {
		t.Fatalf("expected myPipe to be gone after UnloadAll, got %v", err)
	}
	if _, err := r.Get("md"); err != nil {
		t.Fatalf("expected builtin md to survive UnloadAll: %v", err)
	}
}

// Get checks loaded JS pipes before builtins, so a JS pipe can be loaded
// under the same name as a builtin (e.g. "md") and will be returned instead
// of it.
func TestRegistry_Get_JSPipeSameNameAsBuiltin_JSPipeWins(t *testing.T) {
	cfg := newTestRegistryConfig(t, "md", `export function transform(input) { return "custom-md"; }`)
	r := newTestRegistry(t, cfg)

	if err := r.Load("md"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	pipe, err := r.Get("md")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	result, err := pipe.Transform(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if string(result.Content) != "custom-md" {
		t.Fatalf("expected the loaded JS pipe to take precedence over the builtin, got %q", result.Content)
	}
}
