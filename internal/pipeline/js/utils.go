package js

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
)

func argMustString(arg goja.Value) (string, error) {
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
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

func argBytes(vm *goja.Runtime, val goja.Value) ([]byte, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, fmt.Errorf("expected string or Buffer, got %v", val)
	}
	return buffer.Bytes(vm, val), nil
}

// mustStringArg extracts a string from a goja.Value, throwing a JS TypeError if it fails.
func mustStringArg(vm *goja.Runtime, fn string, arg goja.Value) string {
	s, err := argMustString(arg)
	if err != nil {
		panic(vm.NewTypeError("%s: first argument must be a string: %v", fn, err))
	}
	return s
}

// cleanPath normalises a relative path for use with fs.FS:
// strips leading slashes (fs.FS uses unrooted paths) and cleans dots.
func cleanPath(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	for len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	if p == "" {
		return "."
	}
	return p
}

// isNotExist reports whether an error indicates a missing file,
// covering both os and io/fs sentinel errors.
func isNotExist(err error) bool {
	return os.IsNotExist(err) || err == fs.ErrNotExist
}

// newJSDate constructs a JS Date from a Go time.Time using Unix milliseconds.
func newJSDate(vm *goja.Runtime, t time.Time) goja.Value {
	date, err := vm.New(vm.Get("Date").ToObject(vm), vm.ToValue(t.UnixMilli()))
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("construct Date: %w", err)))
	}
	return date
}

func GetSafe(vm *goja.Runtime, obj *goja.Object, key string) (goja.Value, *goja.Exception) {
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
