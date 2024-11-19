package api

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
)

type FsApi struct {
	root string
}

func NewFsApi(root string) (*FsApi, error) {
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(absRoot); os.IsNotExist(err) {
		return nil, fmt.Errorf("root directory '%s' does not exist", absRoot)
	}

	return &FsApi{root: absRoot}, nil
}

func (fs *FsApi) resolvePath(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}
	absPath := filepath.Join(fs.root, cleanPath)

	if !fs.isWithinRoot(absPath) {
		return "", fmt.Errorf("not allowed: path %s is outside of the root directory", path)
	}

	return absPath, nil
}

func (fs *FsApi) isWithinRoot(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absPath, fs.root)
}

func (fs *FsApi) GetModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()

	module.Set("bytes", func(call goja.FunctionCall) goja.Value { return fs.bytes(call, vm) })
	module.Set("text", func(call goja.FunctionCall) goja.Value { return fs.text(call, vm) })
	module.Set("json", func(call goja.FunctionCall) goja.Value { return fs.json(call, vm) })
	module.Set("write", func(call goja.FunctionCall) goja.Value { return fs.write(call, vm) })
	module.Set("append", func(call goja.FunctionCall) goja.Value { return fs.append(call, vm) })
	module.Set("isFile", func(call goja.FunctionCall) goja.Value { return fs.isFile(call, vm) })
	module.Set("isDirectory", func(call goja.FunctionCall) goja.Value { return fs.isDirectory(call, vm) })
	module.Set("createDirectory", func(call goja.FunctionCall) goja.Value { return fs.createDirectory(call, vm) })
	module.Set("delete", func(call goja.FunctionCall) goja.Value { return fs.delete(call, vm) })
	module.Set("deleteAll", func(call goja.FunctionCall) goja.Value { return fs.deleteAll(call, vm) })
	module.Set("list", func(call goja.FunctionCall) goja.Value { return fs.list(call, vm) })
	module.Set("walk", func(call goja.FunctionCall) goja.Value { return fs.walk(call, vm) })
	module.Set("size", func(call goja.FunctionCall) goja.Value { return fs.size(call, vm) })
	module.Set("lastModified", func(call goja.FunctionCall) goja.Value { return fs.lastModified(call, vm) })
	module.Set("copy", func(call goja.FunctionCall) goja.Value { return fs.copy(call, vm) })
	module.Set("copyAll", func(call goja.FunctionCall) goja.Value { return fs.copyAll(call, vm) })
	module.Set("move", func(call goja.FunctionCall) goja.Value { return fs.move(call, vm) })
	module.Set("moveAll", func(call goja.FunctionCall) goja.Value { return fs.moveAll(call, vm) })

	return module
}

func (fs *FsApi) getStringArg(arg goja.Value) (string, error) {
	if arg == nil || goja.IsUndefined(arg) {
		return "", fmt.Errorf("argument is required")
	}
	if str, ok := arg.Export().(string); ok {
		return str, nil
	}
	return "", fmt.Errorf("argument must be a string")
}

func (fs *FsApi) getByteArg(vm *goja.Runtime, arg goja.Value) ([]byte, error) {
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

	return nil, fmt.Errorf("data must be a Uint8Array, ArrayBuffer or string")
}

func (fs *FsApi) bytes(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	return fs.readFile(call, vm, false)
}

func (fs *FsApi) text(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	return fs.readFile(call, vm, true)
}

func (fs *FsApi) json(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		filePath, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		filePath, err = fs.resolvePath(filePath)
		if err != nil {
			reject(err)
			return
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				resolve(goja.Undefined())
			} else {
				reject(err)
			}
			return
		}

		var result interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			reject(err)
			return
		}
		resolve(vm.ToValue(result))
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) readFile(call goja.FunctionCall, vm *goja.Runtime, asText bool) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		filePath, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		filePath, err = fs.resolvePath(filePath)
		if err != nil {
			reject(err)
			return
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				resolve(goja.Undefined())
			} else {
				reject(err)
			}
			return
		}

		if asText {
			resolve(vm.ToValue(string(data)))
		} else {
			arrayBuffer := vm.NewArrayBuffer(data)
			uintArray, err := vm.New(vm.Get("Uint8Array"), vm.ToValue(arrayBuffer))
			if err == nil {
				resolve(uintArray)
			} else {
				reject(err)
			}
		}
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) write(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		filePath, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		filePath, err = fs.resolvePath(filePath)
		if err != nil {
			reject(err)
			return
		}

		data, err := fs.getByteArg(vm, call.Argument(1))
		if err != nil {
			reject(err)
			return
		}

		err = os.WriteFile(filePath, data, 0644)
		if err != nil {
			reject(err)
			return
		}

		resolve(goja.Undefined())
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) append(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		filePath, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		filePath, err = fs.resolvePath(filePath)
		if err != nil {
			reject(err)
			return
		}

		data, err := fs.getByteArg(vm, call.Argument(1))
		if err != nil {
			reject(err)
			return
		}

		file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			reject(err)
			return
		}
		defer file.Close()

		if _, err := file.Write(data); err != nil {
			reject(err)
			return
		}

		resolve(goja.Undefined())
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) isFile(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		filePath, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		filePath, err = fs.resolvePath(filePath)
		if err != nil {
			reject(err)
			return
		}

		info, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				resolve(vm.ToValue(false))
			} else {
				reject(err)
			}
			return
		}

		resolve(vm.ToValue(info.Mode().IsRegular()))
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) isDirectory(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		dirPath, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		dirPath, err = fs.resolvePath(dirPath)
		if err != nil {
			reject(err)
			return
		}

		info, err := os.Stat(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				resolve(vm.ToValue(false))
			} else {
				reject(err)
			}
			return
		}

		resolve(vm.ToValue(info.IsDir()))
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) createDirectory(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		dirPath, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		dirPath, err = fs.resolvePath(dirPath)
		if err != nil {
			reject(err)
			return
		}

		err = os.MkdirAll(dirPath, 0755)
		if err != nil {
			reject(err)
			return
		}

		resolve(goja.Undefined())
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) delete(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		path, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		path, err = fs.resolvePath(path)
		if err != nil {
			reject(err)
			return
		}

		err = os.Remove(path)
		if err != nil {
			if os.IsNotExist(err) {
				resolve(vm.ToValue(false))
			} else {
				reject(err)
			}
			return
		}

		resolve(vm.ToValue(true))
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) deleteAll(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		path, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		path, err = fs.resolvePath(path)
		if err != nil {
			reject(err)
			return
		}

		err = os.RemoveAll(path)
		if err != nil {
			if os.IsNotExist(err) {
				resolve(vm.ToValue(false))
			} else {
				reject(err)
			}
			return
		}

		resolve(vm.ToValue(true))
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) list(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	dirPath, err := fs.getStringArg(call.Argument(0))
	if err != nil {
		panic(vm.ToValue(err))
	}

	dirPath, err = fs.resolvePath(dirPath)
	if err != nil {
		return vm.ToValue(err)
	}

	iterable := vm.NewObject()
	iterable.Set("Symbol.asyncIterator", iterable)

	files, err := os.ReadDir(dirPath)
	if err != nil {
		panic(vm.ToValue(err))
	}

	index := 0

	nextFunc := func(call goja.FunctionCall) goja.Value {
		promise, resolve, _ := vm.NewPromise()

		go func() {
			if index >= len(files) {
				// No more files
				result := vm.NewObject()
				result.Set("done", true)
				resolve(result)
				return
			}

			file := files[index]
			index++

			entry := vm.NewObject()
			entry.Set("name", file.Name())
			entry.Set("isDirectory", file.IsDir())
			entry.Set("isFile", file.Type().IsRegular())
			entry.Set("isSymlink", file.Type()&os.ModeSymlink != 0)

			result := vm.NewObject()
			result.Set("value", entry)
			result.Set("done", false)

			resolve(result)
		}()

		return vm.ToValue(promise)
	}

	iterable.Set("next", nextFunc)
	return iterable
}

func (fs *FsApi) walk(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	rootPath, err := fs.getStringArg(call.Argument(0))
	if err != nil {
		panic(vm.ToValue(err))
	}

	rootPath, err = fs.resolvePath(rootPath)
	if err != nil {
		return vm.ToValue(err)
	}

	iterable := vm.NewObject()
	iterable.Set("Symbol.asyncIterator", iterable)

	paths := []string{}
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		panic(vm.ToValue(err))
	}

	index := 0

	nextFunc := func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()

		go func() {
			if index >= len(paths) {
				// No more entries
				result := vm.NewObject()
				result.Set("done", true)
				resolve(result)
				return
			}

			path := paths[index]
			index++

			info, err := os.Lstat(path)
			if err != nil {
				reject(err)
				return
			}

			entry := vm.NewObject()
			relPath, _ := filepath.Rel(rootPath, path)
			entry.Set("path", relPath)
			entry.Set("name", info.Name())
			entry.Set("isDirectory", info.IsDir())
			entry.Set("isFile", info.Mode().IsRegular())
			entry.Set("isSymlink", info.Mode()&os.ModeSymlink != 0)
			entry.Set("depth", len(filepath.SplitList(relPath))-1)

			result := vm.NewObject()
			result.Set("value", entry)
			result.Set("done", false)

			resolve(result)
		}()

		return vm.ToValue(promise)
	}

	iterable.Set("next", nextFunc)
	return iterable
}

func (fs *FsApi) size(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		filePath, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		filePath, err = fs.resolvePath(filePath)
		if err != nil {
			reject(err)
			return
		}

		info, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				resolve(goja.Undefined())
			} else {
				reject(err)
			}
			return
		}

		resolve(vm.ToValue(info.Size()))
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) lastModified(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		path, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		path, err = fs.resolvePath(path)
		if err != nil {
			reject(err)
			return
		}

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				resolve(goja.Undefined())
			} else {
				reject(err)
			}
			return
		}

		modTime := info.ModTime()
		resolve(vm.ToValue(modTime))
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) copy(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		source, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}
		source, err = fs.resolvePath(source)
		if err != nil {
			reject(err)
			return
		}

		destination, err := fs.getStringArg(call.Argument(1))
		if err != nil {
			reject(err)
			return
		}
		destination, err = fs.resolvePath(destination)
		if err != nil {
			reject(err)
			return
		}

		err = fs.copyFile(source, destination)
		if err != nil {
			reject(err)
			return
		}

		resolve(goja.Undefined())
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) copyAll(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		source, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		source, err = fs.resolvePath(source)
		if err != nil {
			reject(err)
			return
		}

		destination, err := fs.getStringArg(call.Argument(1))
		if err != nil {
			reject(err)
			return
		}
		destination, err = fs.resolvePath(destination)
		if err != nil {
			reject(err)
			return
		}

		err = fs.copyDir(source, destination)
		if err != nil {
			reject(err)
			return
		}

		resolve(goja.Undefined())
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) move(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		source, err := fs.getStringArg(call.Argument(0))
		if err != nil {
			reject(err)
			return
		}

		source, err = fs.resolvePath(source)
		if err != nil {
			reject(err)
			return
		}

		destination, err := fs.getStringArg(call.Argument(1))
		if err != nil {
			reject(err)
			return
		}
		destination, err = fs.resolvePath(destination)
		if err != nil {
			reject(err)
			return
		}

		err = os.Rename(source, destination)
		if err != nil {
			reject(err)
			return
		}

		resolve(goja.Undefined())
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) moveAll(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	// Same as move since os.Rename works for directories
	return fs.move(call, vm)
}

func (fs *FsApi) copyFile(source, destination string) error {
	srcFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func (fs *FsApi) copyDir(source, dest string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(source, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			err = fs.copyDir(srcPath, destPath)
			if err != nil {
				return err
			}
		} else {
			err = fs.copyFile(srcPath, destPath)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
