package api

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/dop251/goja"
	log "github.com/sirupsen/logrus"
)

type FsApi struct {
	root string
}

func NewFsApi(root string) (*FsApi, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(absRoot); os.IsNotExist(err) {
		return nil, errors.New("root directory does not exist")
	}

	return &FsApi{root: absRoot}, nil
}

func (fs *FsApi) resolvePath(relPath string) (string, error) {
	absPath, err := filepath.Abs(filepath.Join(fs.root, relPath))
	if err != nil {
		return "", err
	}

	// TODO hasprefix is deprecated
	if !filepath.HasPrefix(absPath, fs.root) {
		return "", errors.New("access outside root directory is not allowed")
	}

	return absPath, nil
}

func (fs *FsApi) ReadFile(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	path := call.Argument(0).String()

	promise, resolve, reject := vm.NewPromise()

	resolvedPath, err := fs.resolvePath(path)
	log.Info(resolvedPath)
	if err != nil {
		reject(err.Error())
		log.Infof("error: %s", err)
		// TODO do i return the error?
		return vm.ToValue(promise)
	}

	go func() {
		// data, err := os.ReadFile(resolvedPath)
		// log.Infof("data: %s", data)
		// TODO loop
		// env.loop.RunOnLoop(func(vm *goja.Runtime) {
		// 	if err != nil {
		// 		reject(vm.ToValue(err.Error()))
		// 	} else {
		// 		resolve(vm.ToValue(string(data)))
		// 	}
		// })
	}()

	return vm.ToValue(promise)
}

func (fs *FsApi) GetModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	// fs.Set("mkdir", func(call goja.FunctionCall) goja.Value { return mkdir(call, vm) })
	module.Set("readFile", func(call goja.FunctionCall) goja.Value { return fs.ReadFile(call, vm) })
	// fs.Set("readTextFile", func(call goja.FunctionCall) goja.Value { return readTextFile(call, vm) })
	// fs.Set("remove", func(call goja.FunctionCall) goja.Value { return remove(call, vm) })
	// fs.Set("stat", func(call goja.FunctionCall) goja.Value { return stat(call, vm) })
	// fs.Set("writeFile", func(call goja.FunctionCall) goja.Value { return writeFile(call, vm) })
	// fs.Set("writeTextFile", func(call goja.FunctionCall) goja.Value { return writeTextFile(call, vm) })

	return module
}
