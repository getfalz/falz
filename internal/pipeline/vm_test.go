package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCompileBundle_ValidProgram_Compiles(t *testing.T) {
	src := `var __pipe__ = { transform: function(input) { return "ok"; } };`
	if _, err := compileBundle("test", src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileBundle_SyntaxError_ReturnsError(t *testing.T) {
	_, err := compileBundle("test", `function( { }`)
	if err == nil {
		t.Fatalf("expected an error for invalid JS syntax")
	}
}

func newTestVMWorker(t *testing.T, src string) *vmWorker {
	t.Helper()
	prg, err := compileBundle("test", src)
	if err != nil {
		t.Fatalf("compileBundle: %v", err)
	}
	w, err := newVMWorker(context.Background(), vmConfig{name: "test"}, prg)
	if err != nil {
		t.Fatalf("newVMWorker: %v", err)
	}
	t.Cleanup(w.close)
	return w
}

func TestNewVMWorker_MissingPipeGlobal_ReturnsError(t *testing.T) {
	prg, err := compileBundle("test", `var somethingElse = 1;`)
	if err != nil {
		t.Fatalf("compileBundle: %v", err)
	}
	if _, err := newVMWorker(context.Background(), vmConfig{name: "test"}, prg); err == nil {
		t.Fatalf("expected an error when the bundle doesn't set __pipe__")
	}
}

func TestNewVMWorker_TransformNotAFunction_ReturnsError(t *testing.T) {
	prg, err := compileBundle("test", `var __pipe__ = { transform: "not a function" };`)
	if err != nil {
		t.Fatalf("compileBundle: %v", err)
	}
	if _, err := newVMWorker(context.Background(), vmConfig{name: "test"}, prg); err == nil {
		t.Fatalf("expected an error when transform is not a function")
	}
}

func TestVMWorker_Transform_StringReturn(t *testing.T) {
	w := newTestVMWorker(t, `var __pipe__ = { transform: function(input) { return "hello " + input.toString(); } };`)
	result, err := w.transform(context.Background(), []byte("world"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Content) != "hello world" {
		t.Fatalf("got %q, want %q", result.Content, "hello world")
	}
}

func TestVMWorker_Transform_BufferReturn(t *testing.T) {
	w := newTestVMWorker(t, `var __pipe__ = { transform: function(input) { return Buffer.from([104, 105]); } };`)
	result, err := w.transform(context.Background(), []byte("x"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Content) != "hi" {
		t.Fatalf("got %q, want %q", result.Content, "hi")
	}
}

func TestVMWorker_Transform_ObjectWithContentAndAlias(t *testing.T) {
	w := newTestVMWorker(t, `var __pipe__ = { transform: function(input) { return { content: "body", alias: "custom-name" }; } };`)
	result, err := w.transform(context.Background(), []byte("x"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Content) != "body" || result.Alias != "custom-name" {
		t.Fatalf("got content=%q alias=%q, want content=%q alias=%q", result.Content, result.Alias, "body", "custom-name")
	}
}

func TestVMWorker_Transform_ObjectMissingContentField_ReturnsError(t *testing.T) {
	w := newTestVMWorker(t, `var __pipe__ = { transform: function(input) { return { alias: "x" }; } };`)
	if _, err := w.transform(context.Background(), []byte("x"), nil); err == nil {
		t.Fatalf("expected an error when the returned object has no content field")
	}
}

func TestVMWorker_Transform_NullReturn_ReturnsError(t *testing.T) {
	w := newTestVMWorker(t, `var __pipe__ = { transform: function(input) { return null; } };`)
	if _, err := w.transform(context.Background(), []byte("x"), nil); err == nil {
		t.Fatalf("expected an error when transform returns null")
	}
}

func TestVMWorker_Transform_NumberReturn_IsStringified(t *testing.T) {
	w := newTestVMWorker(t, `var __pipe__ = { transform: function(input) { return 42; } };`)
	result, err := w.transform(context.Background(), []byte("x"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Content) != "42" {
		t.Fatalf("got %q, want %q", result.Content, "42")
	}
}

func TestVMWorker_Transform_ThrownError_IsWrapped(t *testing.T) {
	w := newTestVMWorker(t, `var __pipe__ = { transform: function(input) { throw new Error("boom"); } };`)
	_, err := w.transform(context.Background(), []byte("x"), nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected the thrown error to be wrapped, got %v", err)
	}
}

func TestVMWorker_Transform_VarsPassedThrough(t *testing.T) {
	w := newTestVMWorker(t, `var __pipe__ = { transform: function(input) { return this.name; } };`)
	result, err := w.transform(context.Background(), []byte("x"), map[string]any{"name": "acme"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Content) != "acme" {
		t.Fatalf("got %q, want %q", result.Content, "acme")
	}
}

func TestVMWorker_Transform_ContextCancelled_ReturnsCtxErr(t *testing.T) {
	w := newTestVMWorker(t, `var __pipe__ = { transform: function(input) { while (true) {} } };`)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := w.transform(ctx, []byte("x"), nil)
	if err == nil {
		t.Fatalf("expected an error when the context is cancelled mid-transform")
	}
}
