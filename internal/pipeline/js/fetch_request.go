package js

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dop251/goja"
)

type Request struct {
	URL      string
	Method   string
	Headers  *Headers
	_headers *goja.Object
	Body     interface{}
	vm       *goja.Runtime
}

func (r *Request) Get(key string) goja.Value {
	switch key {
	case "url":
		return r.vm.ToValue(r.URL)
	case "method":
		return r.vm.ToValue(r.Method)
	case "headers":
		return r._headers
	// TODO fix this
	case "bodyUsed":
		return r.vm.ToValue(false) // Simplified
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

func (r *Request) Set(key string, val goja.Value) bool {
	return false
}

func (r *Request) Has(key string) bool {
	switch key {
	case "url", "method", "headers", "bodyUsed", "clone", "text", "json":
		return true
	default:
		return false
	}
}

func (r *Request) Delete(key string) bool {
	return false
}

func (r *Request) Keys() []string {
	return []string{"url", "method", "headers", "bodyUsed", "clone", "text", "json"}
}

func (r *Request) getRequest(call goja.FunctionCall) *Request {
	if obj, ok := call.This.Export().(*Request); ok {
		return obj
	}
	panic(r.vm.NewTypeError("Method called on incompatible receiver"))
}

func (r *Request) Clone(call goja.FunctionCall) goja.Value {
	r = r.getRequest(call)
	headers := &Headers{
		header: r.Headers.header.Clone(),
		vm:     r.vm,
	}
	headersObj := r.vm.NewDynamicObject(headers)
	clonedRequest := &Request{
		URL:      r.URL,
		Method:   r.Method,
		Headers:  headers,
		_headers: headersObj,
		Body:     r.Body,
		vm:       r.vm,
	}
	obj := r.vm.NewDynamicObject(clonedRequest)
	return obj
}

func (r *Request) Text(call goja.FunctionCall) goja.Value {
	r = r.getRequest(call)
	promise, resolve, reject := r.vm.NewPromise()
	go func() {
		if bodyStr, ok := r.Body.(string); ok {
			resolve(r.vm.ToValue(bodyStr))
		} else {
			reject(r.vm.ToValue("Body is not a string"))
		}
	}()
	return r.vm.ToValue(promise)
}

func (r *Request) Json(call goja.FunctionCall) goja.Value {
	r = r.getRequest(call)
	promise, resolve, reject := r.vm.NewPromise()
	go func() {
		if bodyStr, ok := r.Body.(string); ok {
			var result interface{}
			err := json.Unmarshal([]byte(bodyStr), &result)
			if err != nil {
				reject(r.vm.ToValue(err.Error()))
			} else {
				resolve(r.vm.ToValue(result))
			}
		} else {
			reject(r.vm.ToValue("Body is not a string"))
		}
	}()
	return r.vm.ToValue(promise)
}

func (r *Request) ToHTTP() (*http.Request, error) {
	var bodyReader io.Reader
	if r.Body != nil {
		switch b := r.Body.(type) {
		case string:
			bodyReader = strings.NewReader(b)
		case []byte:
			bodyReader = bytes.NewReader(b)
		default:
			return nil, fmt.Errorf("unsupported body type")
		}
	}
	httpReq, err := http.NewRequest(r.Method, r.URL, bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header = r.Headers.header
	return httpReq, nil
}

func NewRequestConstructor(vm *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("Request requires at least one argument"))
		}

		url := call.Argument(0).String()

		method := "GET"
		headers := &Headers{
			header: http.Header{},
			vm:     vm,
		}
		var body interface{}

		if len(call.Arguments) > 1 {
			options := call.Argument(1).ToObject(vm)
			if options == nil {
				panic(vm.NewTypeError("Options argument must be an object"))
			}

			// Method
			if m := options.Get("method"); !goja.IsUndefined(m) {
				method = m.String()
			}

			// Headers
			if h := options.Get("headers"); !goja.IsUndefined(h) {
				switch v := h.Export().(type) {
				case *goja.Object:
					headerObj := v
					for _, key := range headerObj.Keys() {
						value := headerObj.Get(key).String()
						headers.header.Set(key, value)
					}
				case map[string]interface{}:
					for key, value := range v {
						headers.header.Set(key, fmt.Sprintf("%v", value))
					}
				case *Headers:
					headers = headers
				default:
					panic(vm.NewTypeError("Invalid headers format"))
				}
			}

			// Body
			if b := options.Get("body"); !goja.IsUndefined(b) {
				body = b.Export()
			}
		}

		headersObj := vm.NewDynamicObject(headers)
		req := &Request{
			URL:      url,
			Method:   method,
			Headers:  headers,
			_headers: headersObj,
			Body:     body,
			vm:       vm,
		}

		obj := vm.NewDynamicObject(req)
		return obj
	}
}
