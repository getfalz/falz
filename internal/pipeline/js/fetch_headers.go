package js

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/dop251/goja"
)

type Headers struct {
	header http.Header
	vm     *goja.Runtime
}

func (h *Headers) getHeaders(call goja.FunctionCall) *Headers {
	if obj, ok := call.This.Export().(*Headers); ok {
		return obj
	}
	panic(h.vm.NewTypeError("Method called on incompatible receiver"))
}

func (h *Headers) Get(key string) goja.Value {
	switch key {
	case "append":
		return h.vm.ToValue(h.Append)
	case "delete":
		return h.vm.ToValue(h.DeleteHeader)
	case "get":
		return h.vm.ToValue(h.GetHeader)
	case "has":
		return h.vm.ToValue(h.HasHeader)
	case "set":
		return h.vm.ToValue(h.SetHeader)
	case "entries":
		return h.vm.ToValue(h.Entries)
	case "keys":
		return h.vm.ToValue(h.KeysMethod)
	case "values":
		return h.vm.ToValue(h.ValuesMethod)
	case "forEach":
		return h.vm.ToValue(h.ForEach)
	case "getSetCookie":
		return h.vm.ToValue(h.GetSetCookie)
	default:
		return goja.Undefined()
	}
}

func (h *Headers) Set(key string, val goja.Value) bool {
	return false
}

func (h *Headers) Has(key string) bool {
	switch key {
	case "append", "delete", "get", "has", "set", "entries", "keys", "values", "forEach", "getSetCookie":
		return true
	default:
		return false
	}
}

func (h *Headers) Delete(key string) bool {
	return false
}

func (h *Headers) Keys() []string {
	return []string{
		"append",
		"delete",
		"get",
		"has",
		"set",
		"entries",
		"keys",
		"values",
		"forEach",
		"getSetCookie",
	}
}

func (h *Headers) Append(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	name := call.Argument(0).String()
	value := call.Argument(1).String()
	h.header.Add(name, value)
	return goja.Undefined()
}

func (h *Headers) DeleteHeader(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	name := call.Argument(0).String()
	h.header.Del(name)
	return goja.Undefined()
}

func (h *Headers) GetHeader(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	name := call.Argument(0).String()
	value := h.header.Get(name)
	if value == "" {
		return goja.Null()
	}
	return h.vm.ToValue(value)
}

func (h *Headers) HasHeader(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	name := call.Argument(0).String()
	_, ok := h.header[name]
	return h.vm.ToValue(ok)
}

func (h *Headers) SetHeader(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	name := call.Argument(0).String()
	value := call.Argument(1).String()
	h.header.Set(name, value)
	return goja.Undefined()
}

func (h *Headers) Entries(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	iterator := h.vm.NewObject()
	index := 0
	entries := []goja.Value{}

	for name, values := range h.header {
		for _, value := range values {
			pair := h.vm.NewArray()
			pair.Set("0", name)
			pair.Set("1", value)
			entries = append(entries, pair)
		}
	}

	next := func(call goja.FunctionCall) goja.Value {
		if index < len(entries) {
			result := h.vm.NewObject()
			result.Set("value", entries[index])
			result.Set("done", false)
			index++
			return result
		}
		result := h.vm.NewObject()
		result.Set("value", goja.Undefined())
		result.Set("done", true)
		return result
	}

	iterator.Set("next", next)
	return iterator
}

func (h *Headers) KeysMethod(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	iterator := h.vm.NewObject()
	index := 0
	keys := []goja.Value{}

	for name := range h.header {
		keys = append(keys, h.vm.ToValue(name))
	}

	next := func(call goja.FunctionCall) goja.Value {
		if index < len(keys) {
			result := h.vm.NewObject()
			result.Set("value", keys[index])
			result.Set("done", false)
			index++
			return result
		}
		result := h.vm.NewObject()
		result.Set("value", goja.Undefined())
		result.Set("done", true)
		return result
	}

	iterator.Set("next", next)
	return iterator
}

func (h *Headers) ValuesMethod(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	iterator := h.vm.NewObject()
	index := 0
	values := []goja.Value{}

	for _, vals := range h.header {
		for _, value := range vals {
			values = append(values, h.vm.ToValue(value))
		}
	}

	next := func(call goja.FunctionCall) goja.Value {
		if index < len(values) {
			result := h.vm.NewObject()
			result.Set("value", values[index])
			result.Set("done", false)
			index++
			return result
		}
		result := h.vm.NewObject()
		result.Set("value", goja.Undefined())
		result.Set("done", true)
		return result
	}

	iterator.Set("next", next)
	return iterator
}

func (h *Headers) ForEach(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	callback := call.Argument(0)
	thisArg := goja.Undefined()
	if len(call.Arguments) > 1 {
		thisArg = call.Argument(1)
	}
	if callable, ok := goja.AssertFunction(callback); ok {
		for name, values := range h.header {
			for _, value := range values {
				_, err := callable(thisArg, h.vm.ToValue(value), h.vm.ToValue(name), call.This)
				if err != nil {
					panic(err)
				}
			}
		}
	} else {
		panic(h.vm.NewTypeError("callback is not a function"))
	}
	return goja.Undefined()
}

func (h *Headers) GetSetCookie(call goja.FunctionCall) goja.Value {
	h = h.getHeaders(call)
	cookies := h.header["Set-Cookie"]
	arr := h.vm.NewArray()
	for idx, cookie := range cookies {
		arr.Set(strconv.Itoa(idx), cookie)
	}
	return arr
}

func NewHeadersConstructor(vm *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		headers := &Headers{
			header: http.Header{},
			vm:     vm,
		}

		if len(call.Arguments) > 0 {
			arg := call.Argument(0).Export()
			switch v := arg.(type) {
			case map[string]interface{}:
				for key, value := range v {
					headers.header.Set(key, fmt.Sprintf("%v", value))
				}
			case []interface{}:
				for _, item := range v {
					if pair, ok := item.([]interface{}); ok && len(pair) == 2 {
						key := fmt.Sprintf("%v", pair[0])
						value := fmt.Sprintf("%v", pair[1])
						headers.header.Add(key, value)
					}
				}
			default:
			}
		}

		obj := vm.NewDynamicObject(headers)
		return obj
	}
}
