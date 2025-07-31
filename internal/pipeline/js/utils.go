package js

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/dop251/goja"
)

func argMustString(arg goja.Value) (string, error) {
	if arg == nil || goja.IsUndefined(arg) {
		return "", fmt.Errorf("argument is nil or undefined")
	}
	if str, ok := arg.Export().(string); ok {
		return str, nil
	}
	return "", fmt.Errorf("argument must be a string")
}

func argMustObject(arg goja.Value) (*goja.Object, error) {
	if arg == nil || goja.IsUndefined(arg) {
		return nil, fmt.Errorf("argument is nil or undefined")
	}
	if o, ok := arg.Export().(*goja.Object); ok {
		return o, nil
	}
	return nil, fmt.Errorf("argument must be an object")
}

func HasArg(vm *goja.Runtime, arg goja.Value) bool {
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg){
		return false
	}
	return true
}
func GetSafe(vm *goja.Runtime, obj *goja.Object, key string) (goja.Value, *goja.Exception){
	var val goja.Value
	exception := vm.Try(func() {
		val = obj.Get(key)
	})

	return val, exception

}
func GetByteArg(vm *goja.Runtime, arg goja.Value) ([]byte, error) {
	if arg == nil || goja.IsUndefined(arg) {
		return nil, fmt.Errorf("argument is nil or undefined")
	}

	arrayBufferConstructor := vm.Get("ArrayBuffer").ToObject(vm)
	if arrayBufferConstructor != nil {
		if vm.InstanceOf(arg, arrayBufferConstructor) {
			buffer, ok := arg.Export().(goja.ArrayBuffer)
			if !ok {
				return nil, errors.New("internal error: failed to convert ArrayBuffer value")
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
				return nil, errors.New("internal error: could not extract underlying ArrayBuffer from Uint8Array")
			}

			offset := int(obj.Get("byteOffset").ToInteger())
			length := int(obj.Get("byteLength").ToInteger())

			raw := buffer.Bytes()
			if offset < 0 || length < 0 || offset+length > len(raw) {
				return nil, fmt.Errorf(
					"invalid Uint8Array range: offset=%d, length=%d, bufferSize=%d",
					offset, length, len(raw),
				)
			}

			return raw[offset : offset+length], nil
		}
	}

	if str, ok := arg.Export().(string); ok {
		return []byte(str), nil
	}

	actual := reflect.TypeOf(arg.Export())
	return nil, fmt.Errorf(
		"unsupported data type: %v (need ArrayBuffer, Uint8Array, or string)",
		actual,
	)
}
