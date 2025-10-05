package pipeline

import "github.com/dop251/goja"

type Vars map[string]any

func (v Vars) Set(key string, value any) {
	v[key] = value
}

func (v Vars) ToGojaObject(vm *goja.Runtime) *goja.Object {
	obj := vm.NewObject()
	for key, value := range v {
		if nestedVars, ok := value.(Vars); ok {
			value = nestedVars.ToGojaObject(vm)
		}
		obj.Set(key, value)
	}
	return obj
}
