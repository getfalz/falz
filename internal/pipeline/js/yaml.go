package js

import (
	"bytes"
	"fmt"

	"github.com/dop251/goja"
	"gopkg.in/yaml.v3"
)

const yamlDefaultIndent = 2

// @ts namespace yaml
//
// @doc
// Parse and serialise YAML documents
func GetYamlModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("parse", func(call goja.FunctionCall) goja.Value { return yamlParse(vm, call) })
	module.Set("stringify", func(call goja.FunctionCall) goja.Value { return yamlStringify(vm, call) })
	return module
}

// @ts function yaml.parse
//
// @doc
// Parses a YAML string and returns the JavaScript value it represents.
//
// Throws a TypeError when the first argument is not a string.
// Throws an Error when the YAML input is malformed.
//
// @param src - string | Buffer - A YAML-encoded string.
// @returns any - The decoded JavaScript value
func yamlParse(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	b, err := argBytes(vm, call.Argument(0))
	if err != nil {
		panic(vm.NewTypeError("yaml.parse: first argument must be a string or Buffer: %v", err))
	}

	var out any
	if err := yaml.Unmarshal(b, &out); err != nil {
		panic(vm.NewGoError(fmt.Errorf("yaml.parse: %w", err)))
	}

	return vm.ToValue(out)
}

// @ts function yaml.stringify
//
// @doc
// Serialises a JavaScript value to a YAML string.
//
// Function-valued properties and Symbol-keyed properties are silently omitted.
//
// The returned string always ends with a newline character.
//
// Throws a TypeError when options has the wrong type.
// Throws an Error when serialisation fails.
//
// @param value    - any              - The JavaScript value to serialise.
// @param options? - StringifyOptions - Optional formatting options.
// @returns string - YAML-encoded string with a trailing newline.
func yamlStringify(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	val := call.Argument(0)
	opts := parseYAMLStringifyOpts(vm, call.Argument(1))

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(opts.Indent)

	if err := encodeRecovering(enc, val.Export()); err != nil {
		panic(vm.NewGoError(fmt.Errorf("yaml.stringify: %w", err)))
	}
	if err := enc.Close(); err != nil {
		panic(vm.NewGoError(fmt.Errorf("yaml.stringify: %w", err)))
	}

	return vm.ToValue(buf.String())
}

// encodeRecovering runs enc.Encode and converts any panic into an error.
// gopkg.in/yaml.v3's Encoder.Encode only recovers its own internal
// yamlError panics; for genuinely unmarshalable values (e.g. a Go func,
// which a JS function value exports as) it panics with a plain string
// instead, which would otherwise crash the process.
func encodeRecovering(enc *yaml.Encoder, v any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return enc.Encode(v)
}

// @ts interface yaml.StringifyOptions
//
// @doc
// Formatting options for yaml.stringify.
//
// @field indent? - number - Number of spaces per indentation level. Defaults to 2.
type yamlStringifyOpts struct {
	Indent int
}

func parseYAMLStringifyOpts(vm *goja.Runtime, val goja.Value) yamlStringifyOpts {
	opts := yamlStringifyOpts{Indent: yamlDefaultIndent}

	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return opts
	}

	obj, ok := val.(*goja.Object)
	if !ok {
		panic(vm.NewTypeError("yaml.stringify: options must be an object, got %T", val.Export()))
	}

	if v := obj.Get("indent"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		switch n := v.Export().(type) {
		case int64:
			if n > 0 {
				opts.Indent = int(n)
			}
		case float64:
			// JS numbers arrive as float64
			if n > 0 {
				opts.Indent = int(n)
			}
		}
	}

	return opts
}
