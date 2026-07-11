package js

import (
	"context"
	"net/http"
	"testing"

	"craeft/internal/utils"

	"github.com/dop251/goja"
)

// newTestRoot returns a *utils.RootFS rooted at a fresh temp dir, used as
// the fs module's root in tests.
func newTestRoot(t *testing.T) *utils.RootFS {
	t.Helper()
	root, err := utils.NewRootFs(t.TempDir())
	if err != nil {
		t.Fatalf("new root fs: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// newTestVM builds a goja.Runtime via SetupGlobals and SetupRequire. root
// is used for require('fs'); pass nil to disable it. policy/transport
// configure require('http').
func newTestVM(t *testing.T, root *utils.RootFS, policy HTTPPolicy, transport *http.Transport) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	vm.SetFieldNameMapper(goja.UncapFieldNameMapper())
	SetupRequire(context.Background(), vm, RuntimeConfig{
		Name:          "test",
		Root:          root,
		HTTPPolicy:    policy,
		HTTPTransport: transport,
	})
	SetupGlobals(vm)
	return vm
}

// runJSFunc runs script (which must define a function named fnName) and
// invokes it with args, returning the JS result or the error goja produced
// (e.g. a *goja.Exception wrapping a thrown TypeError/Error).
func runJSFunc(t *testing.T, vm *goja.Runtime, script, fnName string, args ...any) (goja.Value, error) {
	t.Helper()
	if _, err := vm.RunString(script); err != nil {
		t.Fatalf("run script: %v", err)
	}
	fn, ok := goja.AssertFunction(vm.Get(fnName))
	if !ok {
		t.Fatalf("%s is not a function", fnName)
	}
	gojaArgs := make([]goja.Value, len(args))
	for i, a := range args {
		gojaArgs[i] = vm.ToValue(a)
	}
	return fn(goja.Undefined(), gojaArgs...)
}
