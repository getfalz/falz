package js

import (
	"fmt"
	"io"
	"strings"

	"github.com/dop251/goja"
	"github.com/valyala/fasttemplate"
)

// @ts namespace fasttpl
//
// @doc
// Synchronous template rendering with support for string and function-valued parameters.
func GetFastTplModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("compile", func(call goja.FunctionCall) goja.Value {
		return fastCompile(call, vm)
	})
	return module
}

// @ts function fasttpl.compile
//
// @doc
// Renders a template by substituting tagged placeholders with values from the
// parameters object. Parameter values may be plain strings or zero-argument
// functions that return strings (evaluated lazily per occurrence).
//
// @param template  - string              - Template string, e.g. "Hello {{name}}!"
// @param params    - Record<string, string | (() => string)> - Substitution values.
// @param config?   - fasttpl.Config      - Optional tag delimiters.
// @returns string  - The rendered string.
func fastCompile(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	tpl, err := argMustString(call.Argument(0))
	if err != nil {
		panic(vm.NewTypeError("fasttpl.compile: first argument must be a string: %v", err))
	}

	params, ok := call.Argument(1).(*goja.Object)
	if !ok {
		panic(vm.NewTypeError(
			"fasttpl.compile: second argument must be an object, got %T", call.Argument(1).Export(),
		))
	}

	cfg := parseFtplConfig(vm, call.Argument(2))

	tmpl, err := fasttemplate.NewTemplate(tpl, cfg.StartTag, cfg.EndTag)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fasttpl.compile: invalid template: %w", err)))
	}

	// Build the substitution map. Values can be strings or functions → string.
	parameters := make(map[string]func() string, len(params.Keys()))
	for _, key := range params.Keys() {
		val := params.Get(key)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			continue
		}

		k := key // capture for closures below
		if fn, isFn := goja.AssertFunction(val); isFn {
			parameters[k] = func() string {
				result, err := fn(goja.Undefined())
				if err != nil {
					panic(vm.NewGoError(fmt.Errorf(
						"fasttpl.compile: function for key %q failed: %w", k, err,
					)))
				}
				s, ok := result.Export().(string)
				if !ok {
					panic(vm.NewTypeError(
						"fasttpl.compile: function for key %q must return string, got %T",
						k, result.Export(),
					))
				}
				return s
			}
		} else {
			s, ok := val.Export().(string)
			if !ok {
				panic(vm.NewTypeError(
					"fasttpl.compile: value for key %q must be a string or function, got %T",
					k, val.Export(),
				))
			}
			parameters[k] = func() string { return s }
		}
	}

	var b strings.Builder
	_, err = tmpl.ExecuteFunc(&b, func(w io.Writer, tag string) (int, error) {
		if val, ok := parameters[tag]; ok {
			return io.WriteString(w, val())
		}
		// Unknown tag: emit it verbatim so the template is not silently mangled.
		return fmt.Fprintf(w, "%s%s%s", cfg.StartTag, tag, cfg.EndTag)
	})
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fasttpl.compile: render failed: %w", err)))
	}

	return vm.ToValue(b.String())
}

// @ts interface fasttpl.Config
//
// @field startTag? - string - Opening delimiter. Default: "{{"
// @field endTag?   - string - Closing delimiter. Default: "}}"
type fastTplConfig struct {
	StartTag string
	EndTag   string
}

// parseFtplConfig reads optional config from a JS value.
// Missing or undefined fields fall back to the defaults safely.
func parseFtplConfig(vm *goja.Runtime, val goja.Value) fastTplConfig {
	cfg := fastTplConfig{StartTag: "{{", EndTag: "}}"}

	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return cfg
	}

	opts, ok := val.(*goja.Object)
	if !ok {
		panic(vm.NewTypeError(
			"fasttpl.compile: config must be an object, got %T", val.Export(),
		))
	}

	if v := opts.Get("startTag"); v != nil && !goja.IsUndefined(v) {
		if s, ok := v.Export().(string); ok {
			cfg.StartTag = s
		}
	}
	if v := opts.Get("endTag"); v != nil && !goja.IsUndefined(v) {
		if s, ok := v.Export().(string); ok {
			cfg.EndTag = s
		}
	}

	return cfg
}

// argString extracts a string from a goja.Value.
// Accepts JS string primitives and String objects.
func argString(vm *goja.Runtime, val goja.Value) (string, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return "", fmt.Errorf("argument is %v", val)
	}
	s, ok := val.Export().(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", val.Export())
	}
	return s, nil
}
