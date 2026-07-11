package pipeline

import (
	"context"
	"testing"

	"craeft/internal/utils"

	"github.com/dop251/goja"
)

// newRequireTestVM builds a goja.Runtime wired up exactly like a real
// pipeline VM (console, require(), Buffer/URL globals) via setupVM.
func newRequireTestVM(t *testing.T) *goja.Runtime {
	t.Helper()
	root, err := utils.NewRootFs(t.TempDir())
	if err != nil {
		t.Fatalf("new root fs: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	vm := goja.New()
	w := &vmWorker{vm: vm, cfg: vmConfig{name: "test", root: root}, ctx: context.Background()}
	setupVM(w)
	return vm
}

// Every module name reachable in a real pipeline bundle must resolve to a
// non-undefined object with at least one expected member, confirming
// setupRequire's dispatch table is wired to the right js.* package
// constructor.
func TestRequire_DispatchesKnownModuleNames(t *testing.T) {
	tests := []struct {
		module string
		probe  string
	}{
		{"fs", "typeof m.text === 'function'"},
		{"http", "typeof m.get === 'function'"},
		{"md", "typeof m.toHtml === 'function'"},
		{"fasttpl", "typeof m.compile === 'function'"},
		{"tpl", "typeof m.render === 'function'"},
		{"yaml", "typeof m.parse === 'function' && typeof m.stringify === 'function'"},
	}

	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			vm := newRequireTestVM(t)
			v, err := vm.RunString(`(function() { const m = require('` + tt.module + `'); return ` + tt.probe + `; })()`)
			if err != nil {
				t.Fatalf("require(%q): %v", tt.module, err)
			}
			if !v.ToBoolean() {
				t.Fatalf("require(%q) did not resolve to the expected module shape", tt.module)
			}
		})
	}
}

func TestRequire_UnknownModuleName_ReturnsUndefined(t *testing.T) {
	vm := newRequireTestVM(t)
	v, err := vm.RunString(`require('does-not-exist')`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !goja.IsUndefined(v) {
		t.Fatalf("expected require() of an unknown module to return undefined, got %v", v)
	}
}
