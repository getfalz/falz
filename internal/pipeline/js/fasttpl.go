package js

import (
	"fmt"
	"io"
	"strings"

	"github.com/dop251/goja"
	"github.com/valyala/fasttemplate"
)

// @ts namespace fasttpl
func GetFastTplModule(ev *EventLoop) *goja.Object {
	module := ev.vm.NewObject()
	module.Set("compile", func(call goja.FunctionCall) goja.Value { return FastCompile(call, ev) })

	return module
}

// @ts function fasttpl.compile
//
// @doc
// Renders a template using the provided variables.
//
// @param template - string - A template string with embedded dynamic content (e.g., "<%= name %>").
// @param variables - Record<string, any> - An object (map) containing key-value pairs to replace placeholders in the template.
// @param config? - Config - An object (map) containing key-value pairs to replace placeholders in the template.
// @returns Promise<string> - A promise that resolves to the rendered string with variables replaced.
func FastCompile(call goja.FunctionCall, ev *EventLoop) goja.Value {
	promise, resolve, reject := ev.vm.NewPromise()

	arg0 := call.Argument(0)
	arg1 := call.Argument(1)
	arg2 := call.Argument(2)

	go func() {
		tpl, err := GetByteArg(ev.vm, arg0)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError(
					"First argument to fasttpl.compile must be a string: %w", err,
				))
			})
			return
		}

		params, ok := arg1.(*goja.Object)
		if !ok {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError(
					"Second argument to fasttpl.compile must be an object, got '%v'", arg1.Export(),
				))
			})
			return
		}

		config, err := getFtplConfig(ev.vm, arg2)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(vm.NewTypeError("Third argument to fasttpl.compile could not be parsed: %w", err))
			})
			return
		}

		tmpl, err := fasttemplate.NewTemplate(string(tpl), config.StartTag, config.EndTag)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(vm.NewTypeError("Error creating template: %w", err))
			})
			return
		}

		var keys []string
		if exc := ev.vm.Try(func() {
			keys = params.Keys()
		}); exc != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(exc)
			})
			return
		}

		parameters := make(map[string]func() string)
		for _, currKey := range keys {
			key := currKey

			val, exc := GetSafe(ev.vm, params, key)
			if exc != nil {
				ev.RunOnLoop(func(vm *goja.Runtime) {
					reject(vm.NewTypeError("could not get value of %q, %s", key, exc.Error()))
				})
				return
			}

			if fn, isFn := goja.AssertFunction(val); isFn {
				f := fn
				parameters[key] = func() string {
					result, err := f(goja.Undefined())
					if err != nil {
						ev.RunOnLoop(func(vm *goja.Runtime) {
							reject(ev.vm.NewTypeError(
								"error calling function for key %q: %s", key, err.Error(),
							))
						})
						return ""
					}
					str, ok := result.Export().(string)
					if !ok {
						ev.RunOnLoop(func(vm *goja.Runtime) {
							reject(vm.NewTypeError(
								"function for key %q must return a string", key,
							))
						})
						return ""
					}
					return str
				}
			} else {
				str, ok := val.Export().(string)
				if !ok {
					ev.RunOnLoop(func(vm *goja.Runtime) {
						reject(vm.NewTypeError(
							"value for key %q must be a string or a function returning string", key,
						))
					})
					return
				}
				s := str
				parameters[key] = func() string {
					return s
				}
			}
		}

		var builder strings.Builder
		_, err = tmpl.ExecuteFunc(&builder, func(w io.Writer, tag string) (int, error) {
			if value, ok := parameters[tag]; ok {
				return w.Write([]byte(value()))
			}
			return fmt.Fprintf(w, "%s%s%s", config.StartTag, tag, config.EndTag)
		})

		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(vm.NewTypeError("Error rendering template: %w", err))
			})
			return
		}

		ev.RunOnLoop(func(vm *goja.Runtime) {
			resolve(builder.String())
		})
	}()

	return ev.vm.ToValue(promise)
}

// @ts interface fasttpl.Config
//
// @doc
// This config sets which extensions are enabled when rendering Markdown.
//
// @field startTag: string - Opening tag for variable substitution in templates. Defaults to "{{"
// @field endTag: string - Closing tag for variable substitution in templates. Defaults to "}}"
type FastTplApiConfig struct {
	StartTag string
	EndTag   string
}

func getFtplConfig(vm *goja.Runtime, val goja.Value) (*FastTplApiConfig, error) {
	config := &FastTplApiConfig{
		StartTag: "{{",
		EndTag:   "}}",
	}
	if HasArg(vm, val) {
		opts, ok := val.(*goja.Object)
		if !ok {
			return nil, fmt.Errorf("argument expected to be an object, got '%v'", val.Export())
		}

		if exc := vm.Try(func() {
			config.StartTag = opts.Get("startTag").Export().(string)
			config.EndTag = opts.Get("endTag").Export().(string)
		}); exc != nil {
			return nil, fmt.Errorf("error reading template options: %w", exc)
		}
	}

	return config, nil
}
