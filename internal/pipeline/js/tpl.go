package js

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/gobuffalo/plush"
)

func GetTplModule(ev *EventLoop) *goja.Object {
	module := ev.vm.NewObject()
	module.Set("render", func(call goja.FunctionCall) goja.Value { return Render(call, ev) })
	return module
}

// @jsapi
//
// Renders a template using the provided variables.
//
// @param template - The template string to render.
// @param variables - A key-value map containing variables to be used in the template.
// @returns A promise that resolves to the rendered string.
//
// function Render(template: string, variables: Record<string, any>): Promise<string>;
func Render(call goja.FunctionCall, ev *EventLoop) goja.Value {
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
