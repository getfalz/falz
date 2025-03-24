package js

import (
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/dop251/goja"
)

func GetFetchModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("Headers", NewHeadersConstructor(vm))
	module.Set("Request", NewRequestConstructor(vm))
	module.Set("Response", NewResponseConstructor(vm))

	module.Set("fetch", fetch(vm))
	return module
}

func fetch(vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("fetch requires at least one argument"))
		}

		var req *Request

		arg0 := call.Argument(0)
		if arg0.ExportType().Kind() == reflect.String {
			url := arg0.String()
			req = &Request{
				URL:    url,
				Method: "GET",
				Headers: &Headers{
					header: http.Header{},
					vm:     vm,
				},
				vm: vm,
			}
		} else {
			switch v := arg0.Export().(type) {
			case *goja.Object:
				panic(vm.NewTypeError("TODO handle when fetch request is a object"))
			case *Request:
				req = v
			default:
				panic(vm.NewTypeError("Invalid argument"))
			}

			if obj, ok := arg0.Export().(*goja.Object); ok {
				if r, ok := obj.Export().(*Request); ok {
					req = r
				} else {
					panic(vm.NewTypeError("Invalid Request object"))
				}
			} else {
			}
		}

		if len(call.Arguments) > 1 {
			options := call.Argument(1).ToObject(vm)
			if options != nil {
				if m := options.Get("method"); !goja.IsUndefined(m) {
					req.Method = m.String()
				}

				// Headers
				if h := options.Get("headers"); !goja.IsUndefined(h) {
					switch v := h.Export().(type) {
					case map[string]interface{}:
						for key, value := range v {
							req.Headers.header.Set(key, fmt.Sprintf("%v", value))
						}
					case *goja.Object:
						headerObj := v
						for _, key := range headerObj.Keys() {
							value := headerObj.Get(key).String()
							req.Headers.header.Set(key, value)
						}
					default:
						panic(vm.NewTypeError("Invalid headers format"))
					}
				}

				// Body
				if b := options.Get("body"); !goja.IsUndefined(b) {
					req.Body = b.Export()
				}
			}
		}

		promise, resolve, reject := vm.NewPromise()

		go func() {
			httpReq, err := req.ToHTTP()
			if err != nil {
				reject(vm.ToValue(err.Error()))
				return
			}

			client := &http.Client{}

			resp, err := client.Do(httpReq)
			if err != nil {
				reject(vm.ToValue(err.Error()))
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				reject(vm.ToValue(err.Error()))
				return
			}

			headers := &Headers{
				header: resp.Header,
				vm:     vm,
			}
			headersObj := vm.NewDynamicObject(headers)
			res := &Response{
				Status:     resp.StatusCode,
				StatusText: resp.Status,
				Headers:    headers,
				_headers:   headersObj,
				Body:       body,
				URL:        req.URL,
				Type:       "default",
				vm:         vm,
			}
			obj := vm.NewDynamicObject(res)

			resolve(obj)
		}()

		return vm.ToValue(promise)
	}
}
