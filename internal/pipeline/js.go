package pipeline

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"craeft/internal/log"
	"craeft/internal/pipeline/api"

	"github.com/dop251/goja"
	"github.com/rs/zerolog"
	"golang.org/x/term"
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

	name := filepath.Base(config.Path)

	setupEnv(loop.vm, config.Name)

	p, err := goja.Compile(name, code, false)
	if err != nil {
		return nil, err
	}

	env := &JsEnv{config: config, loop: loop, prg: p}
	env.setupRequire()

	_, err = loop.vm.RunProgram(p)
	if err != nil {
		return nil, fmt.Errorf("error running pipeline %s: %s", config.Name, err)
	}

	return env, nil
}

func (env *JsEnv) ToValue(val any) goja.Value {
	return env.loop.vm.ToValue(val)
}

// JSConfig is a configuration for the JavaScripts environment
type JSConfig struct {
	Name   string
	Path   string
	Root   string
	DBPath string
}

func (env *JsEnv) setupRequire() {
	env.loop.vm.Set("require", func(call goja.FunctionCall) goja.Value {
		modName := call.Argument(0).String()

		// New modules should also be added into script.go:loadScript, so that esbuild can ignore them
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

func (env *JsEnv) Call(name string, ctx, vars goja.Value) (goja.Value, error) {
	v := env.loop.vm.Get(name)
	fn, ok := goja.AssertFunction(v)
	if !ok {
		return vars, fmt.Errorf("no %s function in script %s", name, env.config.Path)
	}

	o := v.ToObject(env.loop.vm)
	p := o.Get("constructor").ToObject(env.loop.vm)

	switch p.Get("name").String() {
	case "Function":
		return env.callFunc(fn, name, ctx, vars)
	case "AsyncFunction":
		return env.callAsyncFunc(fn, name, ctx, vars)
	default:
		return nil, fmt.Errorf("unsupported function type %s", p.Get("name").String())
	}
}

func (env *JsEnv) callFunc(fn goja.Callable, name string, ctx, vars goja.Value) (goja.Value, error) {
	result, err := fn(ctx, vars)
	if err != nil {
		return vars, fmt.Errorf("%s return error %s", name, err)
	}

	return result, nil
}

func (env *JsEnv) callAsyncFunc(fn goja.Callable, name string, ctx, vars goja.Value) (goja.Value, error) {
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

func setupEnv(vm *goja.Runtime, pipelineName string) {
	createConsoleObject(vm, pipelineName)
}

func createConsoleObject(vm *goja.Runtime, pipelineName string) {
	console := vm.NewObject()

	// TODO it would be nice to also print from which method it comes (transform/index)

	var writer io.Writer
	writer = &log.LevelWriter{os.Stdout, os.Stderr}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		cwriter := zerolog.ConsoleWriter{
			Out:           os.Stdout,
			FieldsExclude: []string{"pipeline"},
			PartsOrder:    []string{"level", "pipeline", "message"},
		}

		cwriter.FormatMessage = func(i interface{}) string {
			lines := strings.Split(i.(string), "\n")
			var b strings.Builder
			b.WriteString("\n")
			for _, line := range lines {
				b.WriteString("> ")
				b.WriteString(line)
				b.WriteString("\n")
			}
			return b.String()
		}
		cwriter.FormatFieldValue = func(i interface{}) string {
			return fmt.Sprintf("pipeline:%s:", i)
		}

		writer = cwriter
	}

	l := zerolog.New(writer).With().Timestamp().Logger()

	logFn := func(logger *zerolog.Event, call goja.FunctionCall) goja.Value {
		var b strings.Builder
		for i, arg := range call.Arguments {
			b.WriteString(fmt.Sprintf("%v", arg.Export()))
			if i < len(call.Arguments)-1 {
				b.WriteString("\n")
			}
		}
		logger.Str("pipeline", pipelineName).Msg(b.String())
		return goja.Undefined()
	}

	console.Set("log", func(call goja.FunctionCall) goja.Value {
		return logFn(l.Info(), call)
	})
	console.Set("info", func(call goja.FunctionCall) goja.Value {
		return logFn(l.Info(), call)
	})
	console.Set("warn", func(call goja.FunctionCall) goja.Value {
		return logFn(l.Warn(), call)
	})
	console.Set("error", func(call goja.FunctionCall) goja.Value {
		return logFn(l.Error(), call)
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
