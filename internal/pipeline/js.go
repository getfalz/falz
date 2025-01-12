package pipeline

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"craeft/internal/pipeline/api"

	"github.com/dop251/goja"
)

type JsEnv struct {
	loop   *EventLoop
	config *JSConfig
	prg    *goja.Program

	resources []api.Closable
	rm        sync.Mutex
}

// NewJs returns a ready to use JavaScript environment,
// with require() setup, and auxiliary functions like console.log
func NewJs(config *JSConfig, code string) (*JsEnv, error) {
	loop := NewEventLoop()

	loop.vm.SetFieldNameMapper(goja.UncapFieldNameMapper())

	setupEnv(loop.vm)

	name := filepath.Base(config.Path)

	p, err := goja.Compile(name, code, false)
	if err != nil {
		return nil, err
	}

	_, err = loop.vm.RunProgram(p)
	if err != nil {
		return nil, err
	}

	env := &JsEnv{config: config, loop: loop, prg: p}
	env.setupRequire()

	return env, nil
}

func (env *JsEnv) ToValue(val any) goja.Value {
	return env.loop.vm.ToValue(val)
}

// JSConfig is a configuration for the JavaScripts environment
type JSConfig struct {
	Path   string
	Root   string
	DBPath string
}

func (env *JsEnv) setupRequire() {
	env.loop.vm.Set("require", func(call goja.FunctionCall) goja.Value {
		modName := call.Argument(0).String()

		switch modName {
		case "kv":
			kvApi, err := api.NewKVStore(env.config.DBPath)
			if err != nil {
				// TODO don't use panic
				panic(err)
			}
			env.addClosable(kvApi)
			return kvApi.GetModule(env.loop.vm)
		case "fs":
			fsAPI, err := api.NewFsApi(env.config.Root)
			if err != nil {
				// TODO don't use panic
				panic(err)
			}
			return fsAPI.GetModule(env.loop.vm)
		case "http":
			return api.GetFetchModule(env.loop.vm)
		case "md":
			return api.GetMdModule(env.loop.vm)
		case "fasttpl":
			return api.GetFastTplModule(env.loop.vm)
		case "tpl":
			return api.GetTplModule(env.loop.vm)
		}
		return goja.Undefined()
	})
}

func (env *JsEnv) CallFunc(name string, ctx, vars goja.Value) (goja.Value, error) {
	fn, ok := goja.AssertFunction(env.loop.vm.Get(name))
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
	fn, ok := goja.AssertFunction(env.loop.vm.Get(name))
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
	case <-time.After(5 * time.Second):
		env.loop.StopNoWait()
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
			fmt.Print(arg.String())
		}
		fmt.Print("\n")
		return goja.Undefined()
	})

	vm.Set("console", console)
}

func (env *JsEnv) addClosable(resource api.Closable) {
	env.rm.Lock()
	defer env.rm.Unlock()
	env.resources = append(env.resources, resource)
}

func (env *JsEnv) Close() {
	env.rm.Lock()
	defer env.rm.Unlock()
	for _, resource := range env.resources {
		_ = resource.Close()
	}
	env.resources = nil
}
