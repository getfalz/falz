package js

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

func argBytes(vm *goja.Runtime, val goja.Value) ([]byte, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, fmt.Errorf("expected string or Buffer, got %v", val)
	}
	return buffer.Bytes(vm, val), nil
}

func argString(vm *goja.Runtime, val goja.Value) (string, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return "", fmt.Errorf("expected string or Buffer, got %v", val)
	}
	if s, ok := val.Export().(string); ok {
		return s, nil
	}
	b, err := argBytes(vm, val)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// mustStringArg extracts a string from a goja.Value, throwing a JS TypeError if it fails.
func mustStringArg(vm *goja.Runtime, fn string, val goja.Value) string {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		panic(vm.NewTypeError("expected string, got %v", val))
	}
	s, ok := val.Export().(string)
	if !ok {
		panic(vm.NewTypeError("%s: argument must be a string got %s", fn, val.ExportType()))
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
