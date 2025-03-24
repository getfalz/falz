package js

import (
	"fmt"

	"github.com/dop251/goja"
)

// TODO rename to argMustString
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

func GetByteArg(vm *goja.Runtime, arg goja.Value) ([]byte, error) {
	if arg == nil || goja.IsUndefined(arg) {
		return nil, fmt.Errorf("data argument is required")
	}

	// TODO try this: b := obj.Get("buffer").Export().(goja.ArrayBuffer).Bytes()
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

	return nil, fmt.Errorf("data must be a Uint8Array, ArrayBuffer or string")
}
