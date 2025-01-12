package api

import (
	"github.com/dop251/goja"
	"github.com/gobuffalo/plush"
)

func GetTplModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("render", render(vm))
	return module
}

func render(vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()

		go func() {
			tpl, err := getByteArg(vm, call.Argument(0))
			if err != nil {
				reject(err)
				return
			}
			var vars map[string]any
			err = vm.ExportTo(call.Argument(1), &vars)
			if err != nil {
				reject(err)
				return
			}
			ctx := plush.NewContextWith(vars)
			res, err := plush.Render(string(tpl), ctx)
			if err != nil {
				reject(err)
				return
			}
			resolve(res)
		}()

		return vm.ToValue(promise)
	}
}
