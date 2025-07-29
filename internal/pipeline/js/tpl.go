package js

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/gobuffalo/plush"
)

// @ts namespace tpl
//
// @doc
// A utility module for rendering and manipulating templates using the Plush system.
//
// This module provides various functions that support rendering templates,
// working with variables, and performing other templating tasks.
// It uses the Plush template engine from Go (`github.com/gobuffalo/plush`).
//
// Functions available in this module include `render`, which renders a template string
// using the provided variables, and other utilities that may support different use cases.
func GetTplModule(ev *EventLoop) *goja.Object {
	module := ev.vm.NewObject()
	module.Set("render", func(call goja.FunctionCall) goja.Value { return TplRender(call, ev) })
	return module
}

// @ts function tpl.render
//
// @doc
// Renders a template using the provided variables, leveraging the Plush template system.
//
// @param template - string - A template string with embedded dynamic content (e.g., "<%= name %>").
// @param variables - Record<string, any> - An object (map) containing key-value pairs to replace placeholders in the template.
// @returns Promise<string> - A promise that resolves to the rendered string with variables replaced.
//
// @example
// ```javascript
// const result = await tpl.render("Hello, <%= name %>!", { name: "World" });
// console.log(result); // Output: "Hello, World!"
// ```
func TplRender(call goja.FunctionCall, ev *EventLoop) goja.Value {
	promise, resolve, reject := ev.vm.NewPromise()

	arg0 := call.Argument(0)
	arg1 := call.Argument(1)

	go func() {
		tpl, err := argMustString(arg0)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError(
					"First argument to tpl.render must be a string: %v", err,
				))
			})
			return
		}
		var vars map[string]any
		err = ev.vm.ExportTo(arg1, &vars)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError(
					"Second argument to tpl.render must be an object: %v", err,
				))
			})
			return
		}
		ctx := plush.NewContextWith(vars)
		res, err := plush.Render(string(tpl), ctx)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewGoError(fmt.Errorf(
					"tpl.render failed: %v", err,
				)))
			})
			return
		}
		ev.RunOnLoop(func(vm *goja.Runtime) {
			resolve(res)
		})
	}()

	return ev.vm.ToValue(promise)
}
