package api

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/gobuffalo/plush"
)

func GetTplModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("render", func(call goja.FunctionCall) goja.Value { return render(call, vm) })
	return module
}

func render(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	arg0 := call.Argument(0)
	arg1 := call.Argument(1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				reject(fmt.Errorf("panic: %v", r))
				return
			}
		}()

		tpl, err := getByteArg(vm, arg0)
		if err != nil {
			reject(vm.NewTypeError("got tpl: %s, err: %s", tpl, err))
			return
		}
		var vars map[string]any
		err = vm.ExportTo(arg1, &vars)
		if err != nil {
			reject(vm.NewTypeError(err))
			return
		}
		ctx := plush.NewContextWith(vars)
		res, err := plush.Render(string(tpl), ctx)
		if err != nil {
			reject(vm.NewGoError(err))
			return
		}
		resolve(res)
	}()

	return vm.ToValue(promise)
}
