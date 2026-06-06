package js

import (
	"slices"
	"encoding/json"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
)

// @ts class http.Response
//
// @doc
// The response returned by every `http.*` method.
//
// @field status: number
// @field statusText: string
// @field ok: boolean
// @field redirected: boolean
// @field url: string
// @field headers: Headers
// @field text(): string
// @field json(): any
// @field bytes(): Buffer
// @field arrayBuffer(): ArrayBuffer
type Response struct {
	status     int
	statusText string
	headers    *Headers
	body       []byte
	url        string
	redirected bool
	vm         *goja.Runtime
}

var responsePropertyNames = []string{
	"status", "statusText", "ok", "redirected", "url",
	"headers", "text", "json", "bytes", "arrayBuffer",
}

func (r *Response) Get(key string) goja.Value {
	switch key {
	case "status":
		return r.vm.ToValue(r.status)
	case "statusText":
		return r.vm.ToValue(r.statusText)
	case "ok":
		return r.vm.ToValue(r.status >= 200 && r.status < 300)
	case "redirected":
		return r.vm.ToValue(r.redirected)
	case "url":
		return r.vm.ToValue(r.url)
	case "headers":
		return newHeadersObject(r.vm, r.headers)
	case "text":
		return r.vm.ToValue(r.jsText)
	case "json":
		return r.vm.ToValue(r.jsJSON)
	case "bytes":
		return r.vm.ToValue(r.jsBytes)
	case "arrayBuffer":
		return r.vm.ToValue(r.jsArrayBuffer)
	default:
		return goja.Undefined()
	}
}

func (r *Response) Set(string, goja.Value) bool { return false }

func (r *Response) Has(key string) bool {
	return slices.Contains(responsePropertyNames, key)
}

func (r *Response) Delete(string) bool { return false }
func (r *Response) Keys() []string     { return responsePropertyNames }

func (r *Response) jsText(_ goja.FunctionCall) goja.Value {
	return r.vm.ToValue(string(r.body))
}

func (r *Response) jsJSON(_ goja.FunctionCall) goja.Value {
	if len(r.body) == 0 {
		panic(r.vm.NewGoError(fmt.Errorf("Response.json: body is empty")))
	}
	var result any
	if err := json.Unmarshal(r.body, &result); err != nil {
		panic(r.vm.NewGoError(fmt.Errorf("Response.json: %w", err)))
	}
	return r.vm.ToValue(result)
}

func (r *Response) jsBytes(_ goja.FunctionCall) goja.Value {
	return buffer.WrapBytes(r.vm, r.body)
}

func (r *Response) jsArrayBuffer(_ goja.FunctionCall) goja.Value {
	return r.vm.ToValue(r.vm.NewArrayBuffer(r.body))
}

func newResponseObject(vm *goja.Runtime, r *Response) *goja.Object {
	return vm.NewDynamicObject(r)
}
