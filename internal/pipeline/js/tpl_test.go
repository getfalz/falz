package js

import (
	"strings"
	"testing"
)

func TestTpl_Render_SubstitutesVariables(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const tpl = require('tpl');
			return tpl.render("Hello <%= name %>!", { name: "World" });
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "Hello World!" {
		t.Fatalf("got %v, want %q", result.Export(), "Hello World!")
	}
}

func TestTpl_Render_EscapesByDefault(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const tpl = require('tpl');
			return tpl.render("<%= raw %>", { raw: "<b>bold</b>" });
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.Export().(string), "<b>") {
		t.Fatalf("expected <%%= %%> to HTML-escape by default, got %q", result.Export())
	}
}

func TestTpl_Render_ControlFlow(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const tpl = require('tpl');
			return tpl.render("<%= if (show) { %>yes<%} else { %>no<%} %>", { show: true });
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "yes" {
		t.Fatalf("got %v, want %q", result.Export(), "yes")
	}
}

func TestTpl_Render_UndefinedVariable_Throws(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const tpl = require('tpl');
			return tpl.render("<%= missing %>", {});
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for an undefined template variable")
	}
}

func TestTpl_Render_NonStringTemplate_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const tpl = require('tpl');
			return tpl.render(42, {});
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for a non-string/Buffer template")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}

func TestTpl_Render_NonObjectVariables_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const tpl = require('tpl');
			return tpl.render("hi", "not an object");
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for non-object variables")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}

// tpl.render returns its result synchronously as a plain string, not a
// Promise.
func TestTpl_Render_ReturnsPlainStringSynchronously(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const tpl = require('tpl');
			const r = tpl.render("hi", {});
			return typeof r === 'string' && typeof r.then !== 'function';
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToBoolean() != true {
		t.Fatalf("expected tpl.render to return a plain string synchronously")
	}
}
