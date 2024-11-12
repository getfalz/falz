package pipeline

import (
	"fmt"
	"path/filepath"
	"time"

	"craeft/internal/pipeline/api"

	"github.com/dop251/goja"
)

type JsEnv struct {
	vm     *goja.Runtime
	loop   *EventLoop
	config *JSConfig
	prg    *goja.Program
}

// NewJs returns a ready to use JavaScript environment,
// with require() setup, and auxiliary functions like console.log
func NewJs(config *JSConfig, code string) (*JsEnv, error) {
	loop := NewEventLoop()

	// setup js
	setupEnv(loop.vm)
	setupRequire(loop.vm, loop, config)

	name := filepath.Base(config.Path)

	p, err := goja.Compile(name, code, false)
	if err != nil {
		return nil, err
	}

	_, err = loop.vm.RunProgram(p)
	if err != nil {
		return nil, err
	}

	return &JsEnv{vm: loop.vm, config: config, loop: loop, prg: p}, nil
}

// JSConfig is a configuration for the JavaScripts environment
type JSConfig struct {
	Path string
	Root string
}

func setupRequire(vm *goja.Runtime, loop *EventLoop, config *JSConfig) {
	vm.Set("require", func(call goja.FunctionCall) goja.Value {
		modName := call.Argument(0).String()

		if modName == "fs" {
			fsAPI, err := api.NewFsApi(config.Root)
			if err != nil {
				// TODO don't use panic
				panic(err)
			}
			return fsAPI.GetModule(vm)
			// exports := vm.NewObject()
			//
			// // Implement readFile that returns a promise directly
			// exports.Set("readFile", func(call goja.FunctionCall) goja.Value {
			// 	filename := call.Argument(0).String()
			// 	path := filepath.Join(config.Root, filename)
			// 	log.Infof("filename: %s", path)
			//
			// 	// Create and return a new promise
			// 	promise, resolve, reject := vm.NewPromise()
			//
			// 	go func() {
			// 		// Simulate async file read
			// 		waitFor := rand.Intn(3) + 1
			// 		time.Sleep(time.Duration(waitFor) * time.Second)
			//
			// 		data, err := os.ReadFile(path)
			// 		if err != nil {
			// 			reject(vm.ToValue(err.Error()))
			// 		} else {
			// 			resolve(vm.ToValue(fmt.Sprintf("file: %s\nwaited for: %d\n%s", filename, waitFor, data)))
			// 		}
			// 	}()
			//
			// 	return vm.ToValue(promise)
			// })

			// return exports
		}
		return goja.Undefined()
	})
}

func (env *JsEnv) CallFunc(name string, ctx, vars goja.Value) (goja.Value, error) {
	fn, ok := goja.AssertFunction(env.vm.Get(name))
	if !ok {
		return vars, fmt.Errorf("no %s function in script %s", name, env.config.Path)
	}

	result, err := fn(ctx, vars)
	if err != nil {
		return vars, fmt.Errorf("%s return error %s", name, err)
	}

	return result, nil
}

func (env *JsEnv) CallAsyncFunc(name string, ctx, vars goja.Value) (goja.Value, error) {
	fn, ok := goja.AssertFunction(env.vm.Get(name))
	if !ok {
		return nil, fmt.Errorf("no %s function in script %s", name, env.config.Path)
	}

	var result goja.Value
	var err error
	done := make(chan struct{})

	env.loop.Run(func(vm *goja.Runtime) {
		promise, errr := fn(ctx, vars)
		if errr != nil {
			err = fmt.Errorf("%s return error %s", name, errr)
			close(done)
			return
		}

		go func() {
			// if the return is indeed a promise, wait for it
			if p, ok := promise.Export().(*goja.Promise); ok {
				for {
					time.Sleep(time.Millisecond * 100)
					switch p.State() {
					case goja.PromiseStateRejected:
						err = fmt.Errorf("promise rejected: %s", p.Result().String())
						env.loop.StopNoWait()
						close(done)
						return
					case goja.PromiseStateFulfilled:
						result = p.Result()
						env.loop.StopNoWait()
						close(done)
						return
					case goja.PromiseStatePending:
						continue
					default:
						panic("unexpected promise state pending")
					}
				}
			} else {
				close(done)
			}
		}()
	})

	select {
	case <-done:
		if err != nil {
			return nil, err
		}
		return result, nil
		// case <-time.After(5 * time.Second):
		// 	env.loop.StopNoWait()
		// 	return nil, fmt.Errorf("async operation timed out")
	}
}

func setupEnv(vm *goja.Runtime) {
	createConsoleObject(vm)
}

func createConsoleObject(vm *goja.Runtime) {
	console := vm.NewObject()

	console.Set("log", func(call goja.FunctionCall) goja.Value {
		for _, arg := range call.Arguments {
			fmt.Print(arg.String())
		}
		fmt.Print("\n")
		return goja.Undefined()
	})

	vm.Set("console", console)
}
