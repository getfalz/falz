package js

import (
	"slices"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/dop251/goja"
)

// @ts class http.Headers
//
// @doc
//
// Constructed via `new Headers(init?)` where `init` may be:
//   - omitted — empty Headers
//   - another `Headers` instance — shallow copy
//   - a plain object `{ "name": "value", ... }` — one value per name
//   - an array of pairs `[["name", "value"], ...]` — supports multi-values
//
// @field append(name: string, value: string): void
// @field delete(name: string): void
// @field get(name: string): string | null
// @field has(name: string): boolean
// @field set(name: string, value: string): void
// @field entries(): Iterator<[string, string]>
// @field keys(): Iterator<string>
// @field values(): Iterator<string>
// @field forEach(callback: (value: string, name: string, parent: Headers) => void, thisArg?: any): void
// @field getSetCookie(): string[]
type Headers struct {
	header http.Header
	vm     *goja.Runtime
}

var headersMethodNames = []string{
	"append", "delete", "get", "has", "set",
	"entries", "keys", "values", "forEach", "getSetCookie",
}

// Get implements goja.DynamicObject
func (h *Headers) Get(key string) goja.Value {
	switch key {
	case "append":
		return h.vm.ToValue(h.jsAppend)
	case "delete":
		return h.vm.ToValue(h.jsDelete)
	case "get":
		return h.vm.ToValue(h.jsGet)
	case "has":
		return h.vm.ToValue(h.jsHas)
	case "set":
		return h.vm.ToValue(h.jsSet)
	case "entries":
		return h.vm.ToValue(h.jsEntries)
	case "keys":
		return h.vm.ToValue(h.jsKeys)
	case "values":
		return h.vm.ToValue(h.jsValues)
	case "forEach":
		return h.vm.ToValue(h.jsForEach)
	case "getSetCookie":
		return h.vm.ToValue(h.jsGetSetCookie)
	default:
		return goja.Undefined()
	}
}

// Set implements goja.DynamicObject. Headers properties are immutable
func (h *Headers) Set(string, goja.Value) bool { return false }

func (h *Headers) Has(key string) bool {
	return slices.Contains(headersMethodNames, key)
}

func (h *Headers) Delete(string) bool { return false }
func (h *Headers) Keys() []string     { return headersMethodNames }

func (h *Headers) receiver(call goja.FunctionCall) *Headers {
	if obj, ok := call.This.Export().(*Headers); ok {
		return obj
	}
	panic(h.vm.NewTypeError("Headers method called on incompatible receiver"))
}

func (h *Headers) jsAppend(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	if len(call.Arguments) < 2 {
		panic(h.vm.NewTypeError("Headers.append: 2 arguments required"))
	}
	r.header.Add(call.Argument(0).String(), call.Argument(1).String())
	return goja.Undefined()
}

func (h *Headers) jsDelete(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	if len(call.Arguments) < 1 {
		panic(h.vm.NewTypeError("Headers.delete: 1 argument required"))
	}
	r.header.Del(call.Argument(0).String())
	return goja.Undefined()
}

func (h *Headers) jsGet(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	if len(call.Arguments) < 1 {
		panic(h.vm.NewTypeError("Headers.get: 1 argument required"))
	}
	values := r.header.Values(call.Argument(0).String())
	if len(values) == 0 {
		return goja.Null()
	}

	return h.vm.ToValue(strings.Join(values, ", "))
}

func (h *Headers) jsHas(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	if len(call.Arguments) < 1 {
		panic(h.vm.NewTypeError("Headers.has: 1 argument required"))
	}
	_, ok := r.header[http.CanonicalHeaderKey(call.Argument(0).String())]
	return h.vm.ToValue(ok)
}

func (h *Headers) jsSet(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	if len(call.Arguments) < 2 {
		panic(h.vm.NewTypeError("Headers.set: 2 arguments required"))
	}
	r.header.Set(call.Argument(0).String(), call.Argument(1).String())
	return goja.Undefined()
}

func (h *Headers) sortedPairs() [][2]string {
	names := make([]string, 0, len(h.header))
	for name := range h.header {
		names = append(names, name)
	}
	sort.Strings(names)

	pairs := make([][2]string, 0, len(names))
	for _, name := range names {
		for _, value := range h.header[name] {
			pairs = append(pairs, [2]string{name, value})
		}
	}
	return pairs
}

func newIterator[T any](vm *goja.Runtime, items []T, mapper func(T) goja.Value) goja.Value {
	idx := 0
	iter := vm.NewObject()
	iter.Set("next", func(_ goja.FunctionCall) goja.Value {
		result := vm.NewObject()
		if idx < len(items) {
			result.Set("value", mapper(items[idx]))
			result.Set("done", false)
			idx++
		} else {
			result.Set("value", goja.Undefined())
			result.Set("done", true)
		}
		return result
	})
	iter.Set("@@iterator", func(_ goja.FunctionCall) goja.Value { return iter })
	return iter
}

func (h *Headers) jsEntries(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	return newIterator(h.vm, r.sortedPairs(), func(p [2]string) goja.Value {
		arr := h.vm.NewArray()
		arr.Set("0", p[0])
		arr.Set("1", p[1])
		return arr
	})
}

func (h *Headers) jsKeys(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	pairs := r.sortedPairs()
	keys := make([]string, len(pairs))
	for i, p := range pairs {
		keys[i] = p[0]
	}
	return newIterator(h.vm, keys, func(s string) goja.Value { return h.vm.ToValue(s) })
}

func (h *Headers) jsValues(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	pairs := r.sortedPairs()
	values := make([]string, len(pairs))
	for i, p := range pairs {
		values[i] = p[1]
	}
	return newIterator(h.vm, values, func(s string) goja.Value { return h.vm.ToValue(s) })
}

func (h *Headers) jsForEach(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	if len(call.Arguments) < 1 {
		panic(h.vm.NewTypeError("Headers.forEach: callback is required"))
	}
	callback, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		panic(h.vm.NewTypeError("Headers.forEach: callback is not a function"))
	}
	thisArg := goja.Undefined()
	if len(call.Arguments) > 1 {
		thisArg = call.Argument(1)
	}
	for _, p := range r.sortedPairs() {
		if _, err := callback(thisArg, h.vm.ToValue(p[1]), h.vm.ToValue(p[0]), call.This); err != nil {
			panic(err)
		}
	}
	return goja.Undefined()
}

func (h *Headers) jsGetSetCookie(call goja.FunctionCall) goja.Value {
	r := h.receiver(call)
	cookies := r.header.Values("Set-Cookie")
	arr := h.vm.NewArray()
	for i, c := range cookies {
		arr.Set(fmt.Sprintf("%d", i), c)
	}
	return arr
}

// populateHeaders fills dst from a JS init value. Accepts:
//   - *Headers     — copies all values (preserving multi-valued headers)
//   - plain object — one string value per key
//   - pair array   — [[name, value], ...] allows multi-valued headers
//   - *goja.Object — fallback: reads enumerable string keys
func populateHeaders(vm *goja.Runtime, dst http.Header, init goja.Value) error {
	if init == nil || goja.IsUndefined(init) || goja.IsNull(init) {
		return nil
	}

	switch v := init.Export().(type) {
	case *Headers:
		for name, values := range v.header {
			for _, value := range values {
				dst.Add(name, value)
			}
		}
		return nil

	case map[string]interface{}:
		for key, value := range v {
			dst.Set(key, fmt.Sprintf("%v", value))
		}
		return nil

	case []interface{}:
		for i, item := range v {
			pair, ok := item.([]interface{})
			if !ok || len(pair) != 2 {
				return fmt.Errorf("headers[%d]: expected a [name, value] pair", i)
			}
			dst.Add(fmt.Sprintf("%v", pair[0]), fmt.Sprintf("%v", pair[1]))
		}
		return nil
	}

	if obj, ok := init.(*goja.Object); ok {
		for _, key := range obj.Keys() {
			val := obj.Get(key)
			if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
				continue
			}
			dst.Set(key, val.String())
		}
		return nil
	}

	return fmt.Errorf("headers init must be a Headers, plain object, or array of [name, value] pairs")
}

// newHeadersObject wraps h in a goja DynamicObject so JS property access
// dispatches to the Go methods above.
func newHeadersObject(vm *goja.Runtime, h *Headers) *goja.Object {
	return vm.NewDynamicObject(h)
}

// NewHeadersConstructor returns the `new Headers(init?)` constructor function.
func NewHeadersConstructor(vm *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		h := &Headers{header: http.Header{}, vm: vm}
		if len(call.Arguments) > 0 {
			if err := populateHeaders(vm, h.header, call.Argument(0)); err != nil {
				panic(vm.NewTypeError("new Headers: %v", err))
			}
		}
		return newHeadersObject(vm, h)
	}
}
