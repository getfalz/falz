package api

import (
	"encoding/json"
	"net/http"

	"github.com/dop251/goja"
)

type Response struct {
	Status     int
	StatusText string
	Headers    *Headers
	_headers   *goja.Object
	Body       []byte
	URL        string
	Type       string
	vm         *goja.Runtime
}

func (r *Response) Get(key string) goja.Value {
	switch key {
	case "status":
		return r.vm.ToValue(r.Status)
	case "statusText":
		return r.vm.ToValue(r.StatusText)
	case "headers":
		return r._headers
	case "url":
		return r.vm.ToValue(r.URL)
	case "type":
		return r.vm.ToValue(r.Type)
	case "ok":
		return r.vm.ToValue(r.Status >= 200 && r.Status < 300)
	case "redirected":
		return r.vm.ToValue(false)
	// TODO remove this
	case "bodyUsed":
		return r.vm.ToValue(false)
	case "clone":
		return r.vm.ToValue(r.Clone)
	case "text":
		return r.vm.ToValue(r.Text)
	case "json":
		return r.vm.ToValue(r.Json)
	default:
		return goja.Undefined()
	}
}

func (r *Response) Set(key string, val goja.Value) bool {
	return false
}

func (r *Response) Has(key string) bool {
	switch key {
	case "status", "statusText", "headers", "url", "type", "ok", "redirected", "bodyUsed", "clone", "text", "json":
		return true
	default:
		return false
	}
}

func (r *Response) Delete(key string) bool {
	return false
}

func (r *Response) Keys() []string {
	return []string{"status", "statusText", "headers", "url", "type", "ok", "redirected", "bodyUsed", "clone", "text", "json"}
}

func (r *Response) getResponse(call goja.FunctionCall) *Response {
	if obj, ok := call.This.Export().(*Response); ok {
		return obj
	}
	panic(r.vm.NewTypeError("Method called on incompatible receiver"))
}

func (r *Response) Clone(call goja.FunctionCall) goja.Value {
	r = r.getResponse(call)
	clonedResponse := &Response{
		Status:     r.Status,
		StatusText: r.StatusText,
		Headers: &Headers{
			header: r.Headers.header.Clone(),
			vm:     r.vm,
		},
		Body: r.Body,
		URL:  r.URL,
		Type: r.Type,
		vm:   r.vm,
	}
	obj := r.vm.NewDynamicObject(clonedResponse)
	return obj
}

func (r *Response) Text(call goja.FunctionCall) goja.Value {
	r = r.getResponse(call)
	promise, resolve, _ := r.vm.NewPromise()
	go func() {
		resolve(r.vm.ToValue(string(r.Body)))
	}()
	return r.vm.ToValue(promise)
}

func (r *Response) Json(call goja.FunctionCall) goja.Value {
	r = r.getResponse(call)
	promise, resolve, reject := r.vm.NewPromise()
	go func() {
		var result interface{}
		err := json.Unmarshal(r.Body, &result)
		if err != nil {
			reject(r.vm.ToValue(err.Error()))
		} else {
			resolve(r.vm.ToValue(result))
		}
	}()
	return r.vm.ToValue(promise)
}

func NewResponseConstructor(vm *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		var body []byte
		if len(call.Arguments) > 0 {
			arg := call.Argument(0).Export()
			switch v := arg.(type) {
			case string:
				body = []byte(v)
			case []byte:
				body = v
			default:
				panic(vm.NewTypeError("Invalid body type"))
			}
		}

		headers := &Headers{
			header: http.Header{},
			vm:     vm,
		}
		headersObj := vm.NewDynamicObject(headers)
		res := &Response{
			Status:     200,
			StatusText: "OK",
			Headers:    headers,
			_headers:   headersObj,
			Body:       body,
			Type:       "default",
			vm:         vm,
		}

		obj := vm.NewDynamicObject(res)
		return obj
	}
}
