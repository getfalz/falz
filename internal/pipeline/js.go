package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dop251/goja"
	log "github.com/sirupsen/logrus"
)

type JsEnv struct {
	vm     *goja.Runtime
	config *JSConfig
	loop   *EventLoop
	prg    *goja.Program
}

// NewJs returns a ready to use JavaScript environment,
// with require() setup, and auxiliary functions like console.log
func NewJs(config *JSConfig, code string) (*JsEnv, error) {
	// vm := goja.New()
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
			// fsAPI, err := api.NewFsApi(config.Root, loop)
			// if err != nil {
			// 	// TODO don't use panic
			// 	panic(err)
			// }
			// return fsAPI.GetModule(vm)
			exports := vm.NewObject()

			// Implement readFile that returns a promise directly
			exports.Set("readFile", func(call goja.FunctionCall) goja.Value {
				filename := call.Argument(0).String()
				path := filepath.Join(config.Root, filename)
				log.Infof("filename: %s", path)

				// Create and return a new promise
				promise, resolve, reject := vm.NewPromise()

				go func() {
					// Simulate async file read
					time.Sleep(100 * time.Millisecond)

					data, err := os.ReadFile(path)
					loop.RunOnLoop(func(vm *goja.Runtime) {
						if err != nil {
							reject(vm.ToValue(err.Error()))
						} else {
							resolve(vm.ToValue(string(data)))
						}
					})
				}()

				return vm.ToValue(promise)
			})

			return exports
		}
		return goja.Undefined()
	})
}

func (env *JsEnv) CallFunc(name string, ctx, vars goja.Value) (goja.Value, error) {
	fn, ok := goja.AssertFunction(env.vm.Get(name))
	if !ok {
		return vars, fmt.Errorf("No %s function in script %s", name, env.config.Path)
	}

	result, err := fn(ctx, vars)
	if err != nil {
		return vars, fmt.Errorf("%s return error %s", name, err)
	}

	return result, nil
}

func (env *JsEnv) CallAsyncFunc(name string, ctx, vars goja.Value) (goja.Value, error) {
	resultChan := make(chan goja.Value)
	errorChan := make(chan error, 1)

	env.loop.Run(func(vm *goja.Runtime) {
		fn, ok := goja.AssertFunction(env.vm.Get(name))
		if !ok {
			errorChan <- fmt.Errorf("No %s function in script %s", name, env.config.Path)
			return
		}

		promise, err := fn(ctx, vars)
		if err != nil {
			errorChan <- fmt.Errorf("%s return error %s", name, err)
			return
		}

		var result any
		if p, ok := promise.Export().(*goja.Promise); ok {
			log.Infof("promise: %v", p.State())
			for {
				switch p.State() {
				case goja.PromiseStateRejected:
					errorChan <- fmt.Errorf("promise rejected: %s", p.Result().String())
				case goja.PromiseStateFulfilled:
					result = p.Result().Export()
				case goja.PromiseStatePending:
					continue
				default:
					panic("unexpected promise state pending")
				}
			}
		}

		log.Infof("result: %v", result)

		// then, ok := goja.AssertFunction(promise.ToObject(vm).Get("then"))
		// log.Infof("then: %v", then)
		// if !ok {
		// 	errorChan <- fmt.Errorf("No then function in promise")
		// 	return
		// }
		//
		// log.Infof("executing promise")
		// _, err = then(promise, vm.ToValue(func(got goja.Value) {
		// 	log.Infof("Result: %s\n", got.String())
		// 	resultChan <- got
		// }), vm.ToValue(func(err goja.Value) {
		// 	log.Infof("Error: %s\n", err.String())
		// 	errorChan <- fmt.Errorf("promise rejected: %s", err.String())
		// }))
		// if err != nil {
		// 	errorChan <- fmt.Errorf("error setting up promise handlers: %v", err)
		// }
	})

	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return nil, err
	case <-time.After(5 * time.Second): // Add timeout
		return nil, fmt.Errorf("async operation timed out")
	}
}

func setupEnv(vm *goja.Runtime) {
	createConsoleObject(vm)
}

func createConsoleObject(vm *goja.Runtime) {
	console := vm.NewObject()

	console.Set("log", func(call goja.FunctionCall) goja.Value {
		for _, arg := range call.Arguments {
			fmt.Println(arg.String())
		}
		return goja.Undefined()
	})

	vm.Set("console", console)
}
