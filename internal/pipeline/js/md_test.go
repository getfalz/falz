package js

import (
	"strings"
	"testing"
)

func TestMd_ToHtml_BasicMarkdown(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const md = require('md');
			return md.toHtml("# Hello\n\nWorld");
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	html, _ := result.Export().(string)
	if !strings.Contains(html, "<h1>Hello</h1>") || !strings.Contains(html, "<p>World</p>") {
		t.Fatalf("unexpected HTML output: %q", html)
	}
}

func TestMd_ToHtml_AcceptsBufferInput(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const md = require('md');
			return md.toHtml(Buffer.from("# Hi"));
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Export().(string), "<h1>Hi</h1>") {
		t.Fatalf("unexpected HTML output: %v", result.Export())
	}
}

// Parser extensions (e.g. Tables) are opt-in and only take effect when
// explicitly requested in the options object.
func TestMd_ToHtml_ParserExtensionsAreOptIn(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	table := "a|b\n---|---\n1|2"

	withoutTables, err := runJSFunc(t, vm, `
		function run(table) {
			const md = require('md');
			return md.toHtml(table);
		}`, "run", table)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(withoutTables.Export().(string), "<table>") {
		t.Fatalf("expected no <table> without the Tables extension enabled, got %q", withoutTables.Export())
	}

	withTables, err := runJSFunc(t, vm, `
		function run(table) {
			const md = require('md');
			return md.toHtml(table, { parser: { Tables: true } });
		}`, "run", table)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(withTables.Export().(string), "<table>") {
		t.Fatalf("expected <table> with the Tables extension enabled, got %q", withTables.Export())
	}
}

func TestMd_ToHtml_RendererFlagsAffectOutput(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const md = require('md');
			return md.toHtml("[link](https://example.com/x)", { renderer: { NofollowLinks: true } });
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Export().(string), `rel="nofollow"`) {
		t.Fatalf("expected rel=nofollow with NofollowLinks enabled, got %q", result.Export())
	}
}

// md.commonOptions must be directly usable as md.toHtml's options argument.
func TestMd_CommonOptions_IsUsableAsToHtmlOptions(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const md = require('md');
			return md.toHtml("a|b\n---|---\n1|2", md.commonOptions);
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Export().(string), "<table>") {
		t.Fatalf("expected md.commonOptions to enable Tables, got %q", result.Export())
	}
}

func TestMd_ToHtml_MissingArgument_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const md = require('md');
			return md.toHtml();
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error when calling md.toHtml with no arguments")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}

func TestMd_ToHtml_NonObjectOptions_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const md = require('md');
			return md.toHtml("# hi", "not an object");
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for non-object options")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}
