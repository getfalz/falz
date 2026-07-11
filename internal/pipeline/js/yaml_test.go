package js

import (
	"strings"
	"testing"
)

func TestYaml_Parse_DecodesScalarsAndCollections(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			const obj = yaml.parse("name: acme\ncount: 3\ntags:\n  - a\n  - b\n");
			return obj.name + ':' + obj.count + ':' + obj.tags.join(',');
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "acme:3:a,b" {
		t.Fatalf("got %v, want %q", result.Export(), "acme:3:a,b")
	}
}

func TestYaml_Parse_AcceptsBufferInput(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			return yaml.parse(Buffer.from("x: 1")).x;
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToInteger() != 1 {
		t.Fatalf("got %v, want 1", result.Export())
	}
}

func TestYaml_Parse_MalformedYAML_Throws(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			return yaml.parse("a: [1, 2\nb: broken");
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for malformed YAML")
	}
}

func TestYaml_Parse_NonStringArg_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			return yaml.parse(undefined);
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for a missing argument")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}

func TestYaml_Stringify_RoundTripsThroughParse(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			const src = { name: "acme", tags: ["a", "b"] };
			const roundTripped = yaml.parse(yaml.stringify(src));
			return roundTripped.name + ':' + roundTripped.tags.join(',');
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "acme:a,b" {
		t.Fatalf("got %v, want %q", result.Export(), "acme:a,b")
	}
}

func TestYaml_Stringify_EndsWithNewline(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			return yaml.stringify({ a: 1 });
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, _ := result.Export().(string)
	if !strings.HasSuffix(s, "\n") {
		t.Fatalf("expected the output to end with a newline as documented, got %q", s)
	}
}

func TestYaml_Stringify_IndentOption(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			return yaml.stringify({ a: { b: 1 } }, { indent: 4 });
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, _ := result.Export().(string)
	if !strings.Contains(s, "\n    b: 1") {
		t.Fatalf("expected 4-space indentation, got %q", s)
	}
}

func TestYaml_Stringify_DefaultIndentIsTwoSpaces(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			return yaml.stringify({ a: { b: 1 } });
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, _ := result.Export().(string)
	if !strings.Contains(s, "\n  b: 1") {
		t.Fatalf("expected the documented default of 2-space indentation, got %q", s)
	}
}

func TestYaml_Stringify_NonObjectOptions_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			return yaml.stringify({ a: 1 }, "not an object");
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for non-object options")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}

func TestYaml_Stringify_UnmarshalableValue_ThrowsCatchableError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const yaml = require('yaml');
			return yaml.stringify(function() {});
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error when serialising an unmarshalable value")
	}
}
