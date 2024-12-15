package api

import (
	"fmt"
	"io"
	"strings"

	"github.com/dop251/goja"
	"github.com/valyala/fasttemplate"
)

type TplApi struct {
	template   *fasttemplate.Template
	startTag   string
	endTag     string
	parameters map[string]func() string
}

func GetTplModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("Template", NewTplApiConstructor(vm))
	return module
}

func NewTplApiConstructor(vm *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("TemplateApi requires a configuration object"))
		}

		optionsArg := call.Argument(0).ToObject(vm)
		config := TplApiConfig{
			StartTag: "{{",
			EndTag:   "}}",
		}

		if startTag := optionsArg.Get("startTag"); startTag != nil {
			config.StartTag = startTag.String()
		}
		if endTag := optionsArg.Get("endTag"); endTag != nil {
			config.EndTag = endTag.String()
		}

		templateApi := NewTplApi(vm, config)

		return vm.ToValue(templateApi).ToObject(vm)
	}
}

type TplApiConfig struct {
	StartTag string
	EndTag   string
}

func NewTplApi(vm *goja.Runtime, options TplApiConfig) *TplApi {
	return &TplApi{
		startTag:   options.StartTag,
		endTag:     options.EndTag,
		parameters: make(map[string]func() string),
	}
}

func (t *TplApi) Use(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	if len(call.Arguments) < 1 {
		panic(vm.ToValue("Use requires a template string"))
	}

	arg := call.Argument(0)
	templateStr, err := t.getByteArg(vm, arg)
	if err != nil {
		panic(vm.NewTypeError(err))
	}

	tmpl, err := fasttemplate.NewTemplate(string(templateStr), t.startTag, t.endTag)
	if err != nil {
		panic(vm.NewTypeError(fmt.Sprintf("Error creating template: %s", err)))
	}

	t.template = tmpl
	return vm.ToValue(nil)
}

func (t *TplApi) getByteArg(vm *goja.Runtime, arg goja.Value) ([]byte, error) {
	if arg == nil || goja.IsUndefined(arg) {
		return nil, fmt.Errorf("data argument is required")
	}

	arrayBufferConstructor := vm.Get("ArrayBuffer").ToObject(vm)
	if arrayBufferConstructor != nil {
		if vm.InstanceOf(arg, arrayBufferConstructor) {
			buffer, ok := arg.Export().(goja.ArrayBuffer)
			if !ok {
				return nil, fmt.Errorf("failed to convert ArrayBuffer")
			}
			return buffer.Bytes(), nil
		}
	}

	uint8ArrayConstructor := vm.Get("Uint8Array").ToObject(vm)
	if uint8ArrayConstructor != nil {
		if vm.InstanceOf(arg, uint8ArrayConstructor) {
			obj := arg.ToObject(vm)

			bufferProp := obj.Get("buffer")
			bufferObj := bufferProp.ToObject(vm)
			buffer, ok := bufferObj.Export().(goja.ArrayBuffer)
			if !ok {
				return nil, fmt.Errorf("failed to retrieve ArrayBuffer from Uint8Array")
			}

			byteOffsetProp := obj.Get("byteOffset")
			byteLengthProp := obj.Get("byteLength")
			byteOffset := int(byteOffsetProp.ToInteger())
			byteLength := int(byteLengthProp.ToInteger())

			bufferBytes := buffer.Bytes()
			if byteOffset < 0 || byteLength < 0 || byteOffset+byteLength > len(bufferBytes) {
				return nil, fmt.Errorf("invalid byteOffset or byteLength")
			}

			data := bufferBytes[byteOffset : byteOffset+byteLength]
			return data, nil
		}
	}

	if str, ok := arg.Export().(string); ok {
		return []byte(str), nil
	}

	return nil, fmt.Errorf("data must be a Uint8Array, ArrayBuffer, or string")
}

func (t *TplApi) SetParameters(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	if len(call.Arguments) < 1 {
		panic(vm.ToValue("SetParameters requires a parameters object"))
	}

	params := call.Argument(0).ToObject(vm)
	for _, key := range params.Keys() {
		val := params.Get(key)

		if fn, isFunction := goja.AssertFunction(val); isFunction {
			t.parameters[key] = func() string {
				result, err := fn(goja.Undefined())
				if err != nil {
					panic(fmt.Sprintf("Error calling function for key '%s': %v", key, err))
				}
				if str, ok := result.Export().(string); ok {
					return str
				}
				panic(fmt.Sprintf("Function for key '%s' must return a string", key))
			}
		} else {
			if str, ok := val.Export().(string); ok {
				t.parameters[key] = func() string {
					return str
				}
			} else {
				panic(fmt.Sprintf("Value for key '%s' must be a string or a function returning a string", key))
			}
		}
	}

	return vm.ToValue(nil)
}

func (t *TplApi) Render(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	if t.template == nil {
		panic(vm.ToValue("Template not set"))
	}

	var builder strings.Builder
	_, err := t.template.ExecuteFunc(&builder, func(w io.Writer, tag string) (int, error) {
		if value, ok := t.parameters[tag]; ok {
			return w.Write([]byte(value()))
		}
		return fmt.Fprintf(w, "%s%s%s", t.startTag, tag, t.endTag)
	})

	if err != nil {
		panic(vm.ToValue(fmt.Sprintf("Error rendering template: %s", err)))
	}

	return vm.ToValue(builder.String())
}

func (t *TplApi) Get(key string) goja.Value {
	return nil
}

func (t *TplApi) Set(key string, val goja.Value) bool {
	return false
}

func (t *TplApi) Has(key string) bool {
	return false
}

func (t *TplApi) Delete(key string) bool {
	return false
}

func (t *TplApi) Keys() []string {
	return []string{}
}
