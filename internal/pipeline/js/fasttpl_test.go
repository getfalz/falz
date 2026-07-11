package js

import (
	"strings"
	"testing"
)

func TestFastTpl_Compile_SubstitutesStringParams(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fasttpl = require('fasttpl');
			return fasttpl.compile("Hello {{name}}!", { name: "World" });
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "Hello World!" {
		t.Fatalf("got %v, want %q", result.Export(), "Hello World!")
	}
}

func TestFastTpl_Compile_SupportsFunctionValuedParams(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fasttpl = require('fasttpl');
			let calls = 0;
			const r = fasttpl.compile("{{a}}-{{a}}", { a: () => { calls++; return "x" + calls; } });
			return r;
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "x1-x2" {
		t.Fatalf("expected a zero-arg function param to be evaluated lazily per occurrence, got %v", result.Export())
	}
}

func TestFastTpl_Compile_UnknownTagPassesThroughVerbatim(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fasttpl = require('fasttpl');
			return fasttpl.compile("Hi {{unknown}}!", {});
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "Hi {{unknown}}!" {
		t.Fatalf("expected an unmatched tag to be left verbatim, got %v", result.Export())
	}
}

func TestFastTpl_Compile_CustomDelimiters(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fasttpl = require('fasttpl');
			return fasttpl.compile("Hi [name]!", { name: "Bob" }, { startTag: "[", endTag: "]" });
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "Hi Bob!" {
		t.Fatalf("got %v, want %q", result.Export(), "Hi Bob!")
	}
}

func TestFastTpl_Compile_MissingTemplateArg_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const fasttpl = require('fasttpl');
			return fasttpl.compile(undefined, {});
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for a missing template argument")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}

func TestFastTpl_Compile_NonObjectParams_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const fasttpl = require('fasttpl');
			return fasttpl.compile("hi {{x}}", "not an object");
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for non-object params")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}

func TestFastTpl_Compile_NonStringParamValue_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const fasttpl = require('fasttpl');
			return fasttpl.compile("hi {{x}}", { x: 42 });
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for a non-string/function param value")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}

func TestFastTpl_Compile_FunctionReturningNonString_ThrowsError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const fasttpl = require('fasttpl');
			return fasttpl.compile("hi {{x}}", { x: () => 42 });
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error when a param function returns a non-string value")
	}
}

func TestFastTpl_Compile_InvalidConfigType_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const fasttpl = require('fasttpl');
			return fasttpl.compile("hi {{x}}", { x: "y" }, "not an object");
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for a non-object config")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}
