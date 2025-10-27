package js

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"craeft/internal/log"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	"github.com/rs/zerolog"
	"golang.org/x/term"
)

type JsEnv struct {
	loop   *EventLoop
	config *JSConfig
	prg    *goja.Program
	logger zerolog.Logger

	resources []Closable
	rm        sync.Mutex
}

// NewJs returns a ready to use JavaScript environment,
// with require() setup, and auxiliary functions like console.log
func NewJs(config *JSConfig, code string) (*JsEnv, error) {
	l := getLogger(config.Name)

	loop := NewEventLoop()

	setupEnv(loop.vm, config.Name, l)

	name := filepath.Base(config.Path)
	p, err := goja.Compile(name, code, false)
	if err != nil {
		// TODO log error using log.EError
		var ex *goja.Exception
		if errors.As(err, &ex) {
			// ex.Value() is the thrown JS value (could be a SyntaxError, ReferenceError, or any Error subtype)
			// ex.String() prints message + JS stack trace
			fmt.Println("▶ JS Exception:")
			fmt.Println("   Value():", ex.Value())
			fmt.Println("   String():", ex.String())
			return nil, err
		}

		// 2) Next, catch a parser‐level syntax error (before it got wrapped)
		var pe *parser.Error
		if errors.As(err, &pe) {
			fmt.Printf("▶ Parser.Error: %s at %s:%d:%d\n",
				pe.Message,
				pe.Position.Filename,
				pe.Position.Line,
				pe.Position.Column,
			)
			return nil, err
		}

		// 3) Maybe there are other compiler‐phase errors exposed by Goja
		var cref *goja.CompilerReferenceError
		if errors.As(err, &cref) {
			fmt.Printf("▶ ReferenceError: %s at %s:%d:%d\n",
				pe.Message,
				pe.Position.Filename,
				pe.Position.Line,
				pe.Position.Column,
			)
			return nil, err
		}

		// 4) Fallback to whatever else
		fmt.Printf("▶ Unknown error type (%T): %v\n", err, err)
		return nil, err
	}

	env := &JsEnv{config: config, loop: loop, prg: p, logger: l}
	env.setupRequire()

	_, err = loop.vm.RunProgram(p)
	if err != nil {
		fmt.Printf("error running pipeline %s: %s", config.Name, err)
		return nil, fmt.Errorf("error running pipeline %s: %s", config.Name, err)
	}

	loop.Start()

	return env, nil
}

func (env *JsEnv) VM() *goja.Runtime {
	return env.loop.vm
}

func (env *JsEnv) ToValue(val any) goja.Value {
	return env.loop.vm.ToValue(val)
}

// JSConfig is a configuration for the JavaScripts environment
type JSConfig struct {
	Name    string
	Path    string
	Root    string
	DBPath  string
	Timeout time.Duration
}

func (env *JsEnv) setupRequire() {
	env.loop.vm.Set("require", func(call goja.FunctionCall) goja.Value {
		modName := call.Argument(0).String()

		// New modules should also be added into script.go:loadScript, so that esbuild can ignore them
		switch modName {
		case "kv":
			kvApi, err := NewKVStore(env.config.DBPath)
			if err != nil {
				// TODO don't use panic
				panic(err)
			}
			env.addClosable(kvApi)
			return kvApi.GetModule(env.loop.vm)
		case "fs":
			fsAPI, err := NewFsApi(env.config.Root)
			if err != nil {
				// TODO don't use panic
				panic(err)
			}
			return fsAPI.GetModule(env.loop.vm)
		case "http":
			return GetFetchModule(env.loop.vm)
		case "md":
			return GetMdModule(env.loop)
		case "fasttpl":
			return GetFastTplModule(env.loop)
		case "tpl":
			return GetTplModule(env.loop)
		}
		return goja.Undefined()
	})
}

func (env *JsEnv) Call(ctx context.Context, name string, ctxVal, vars goja.Value) (goja.Value, error) {
	v := env.loop.vm.Get(name)
	fn, ok := goja.AssertFunction(v)
	if !ok {
		return vars, fmt.Errorf("no %s function in script %s", name, env.config.Path)
	}

	o := v.ToObject(env.loop.vm)
	p := o.Get("constructor").ToObject(env.loop.vm)

	switch p.Get("name").String() {
	case "Function":
		return env.callFunc(ctx, fn, name, ctxVal, vars)
	case "AsyncFunction":
		return env.callAsyncFunc(ctx, fn, name, ctxVal, vars)
	default:
		return nil, fmt.Errorf("unsupported function type %s", p.Get("name").String())
	}
}

func (env *JsEnv) callFunc(ctx context.Context, fn goja.Callable, name string, ctxVal, vars goja.Value) (goja.Value, error) {
	stop := bindContextCancel(env.loop, env.loop.vm, ctx)
	defer stop()

	result, err := fn(ctxVal, vars)
	if err != nil {
		var ie *goja.InterruptedError
		if errors.As(err, &ie) && ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return vars, fmt.Errorf("%s return error %s", name, err)
	}

	return result, nil
}

func (env *JsEnv) callAsyncFunc(ctx context.Context, fn goja.Callable, name string, ctxVal, vars goja.Value) (goja.Value, error) {
	var (
		result goja.Value
		err    error
		done   = make(chan struct{})
	)

	stop := bindContextCancel(env.loop, env.loop.vm, ctx)
	defer stop()

	env.loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()

				stackStr := string(stack)
				stackLines := strings.Split(stackStr, "\n")

				err = fmt.Errorf("loop.run panic: %v\nStack Trace:\n%s", r, strings.Join(stackLines, "\n"))
				close(done)
			}
		}()

		promiseVal, err := fn(ctxVal, vars)
		if err != nil {
			err = fmt.Errorf("loop.run %s return error %s", name, err)
			close(done)
			return
		}

		if _, ok := promiseVal.Export().(*goja.Promise); !ok {
			result = promiseVal
			close(done)
			return
		}

		var once sync.Once
		resolve := func(call goja.FunctionCall) goja.Value {
			once.Do(func() {
				result = call.Argument(0)
				close(done)
			})
			return goja.Undefined()
		}
		reject := func(call goja.FunctionCall) goja.Value {
			once.Do(func() {
				err = errors.New(call.Argument(0).String())
				close(done)
			})
			return goja.Undefined()
		}

		pObj := promiseVal.ToObject(vm)
		if thenVal := pObj.Get("then"); thenVal != nil {
			if thenFn, ok := goja.AssertFunction(thenVal); ok {
				// promise.then(resolve, reject)
				_, _ = thenFn(pObj, vm.ToValue(resolve), vm.ToValue(reject))
				return
			}
		}

		// fallback in case "then" is not a "real" promise
		vm.Set("__p", promiseVal)
		vm.Set("__goResolve", resolve)
		vm.Set("__goReject", reject)
		_, _ = vm.RunString(`__p && __p.then(__goResolve, __goReject)`)
	})

	select {
	case <-done:
		if err != nil {
			if ctx != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		return result, nil
	case <-ctx.Done():
		env.loop.RunOnLoop(func(vm *goja.Runtime) {
			vm.Interrupt(ctx.Err())
		})
		env.loop.Terminate()
		return nil, ctx.Err()
	case <-time.After(env.config.Timeout):
		env.loop.RunOnLoop(func(vm *goja.Runtime) {
			vm.Interrupt(fmt.Errorf("%s timeout after %s", name, env.config.Timeout))
		})
		return nil, fmt.Errorf("transform timeout after %s", env.config.Timeout)
	}
}

func bindContextCancel(loop *EventLoop, vm *goja.Runtime, ctx context.Context) (stop func()) {
	if ctx == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if vm != nil {
				vm.Interrupt(ctx.Err())
			}
			if loop != nil {
				loop.Terminate()
			}
		case <-done:
			return
		}
	}()

	return func() {
		close(done)
		if vm != nil {
			vm.ClearInterrupt()
		}
	}
}

func setupEnv(vm *goja.Runtime, pipelineName string, l zerolog.Logger) {

	vm.SetFieldNameMapper(goja.UncapFieldNameMapper())

	vm.SetPromiseRejectionTracker(func(p *goja.Promise, op goja.PromiseRejectionOperation) {
		var err string
		val := p.Result()
		if obj, ok := val.(*goja.Object); ok {
			if stack := obj.Get("stack"); stack != nil {
				err = stack.String()
			} else {
				err = val.String()
			}
		} else {
			err = val.String()
		}
		l.Error().Msg(fmt.Sprintf("unhandled promise rejection:\n%s", err))
	})

	createConsoleObject(vm, pipelineName, l)

	// mockup module/exports
	moduleObj := vm.NewObject()
	exportsObj := vm.NewObject()
	moduleObj.Set("exports", exportsObj)
	vm.Set("module", moduleObj)
	vm.Set("exports", exportsObj)
}

func getLogger(pipelineName string) zerolog.Logger {
	// TODO it would be nice to also print from which method it comes (transform/index)
	// TODO when printing to console, the formatting is lost (only for ConsoleWritter)
	// {"level":"info","pipeline":"md3","pipeline":"md3","time":1755434374,"message":"# b_a title\n\n This is a file\n"}

	var writer io.Writer
	writer = &log.LevelWriter{os.Stdout, os.Stderr}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		cwriter := zerolog.ConsoleWriter{
			Out:           os.Stdout,
			FieldsExclude: []string{"pipeline"},
			PartsOrder:    []string{"level", "pipeline", "message"},
		}

		cwriter.FormatMessage = func(i interface{}) string {
			if i == nil || i == "" {
				return ""
			}
			lines := strings.Split(fmt.Sprintf("%s", i), "\n")
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
		cwriter.FormatLevel = func(i any) string {
			switch i {
			case "debug":
				return "[console.dump]"
			case "info":
				return "[console.log]"
			case "warn":
				return "[console.warn]"
			case "error":
				return "[console.error]"
			case zerolog.FatalLevel:
				return "[FATAL]"
			case zerolog.PanicLevel:
				return "[PANIC]"
			default:
				return "[[[" + fmt.Sprintf("%s", i) + "]]]"
			}
		}

		writer = cwriter
	}

	l := zerolog.New(writer).With().Timestamp().Str("pipeline", pipelineName).Logger()
	return l
}

// @ts interface global.Console
// @field log(...data: any[]): void - Logs the provided arguments to the console with info level.
// @field info(...data: any[]): void - Logs the provided arguments to the console with info level.
// @field warn(...data: any[]): void - Logs the provided arguments to the console with warn level.
// @field error(...data: any[]): void - Logs the provided arguments to the console with error level.
// @field dump(...data: any[]): void - Dumps the provided arguments to the console with error level.

// @ts var global.console
// @type Console
// @doc Console provides a simple logging interface for JavaScript pipelines.
func createConsoleObject(vm *goja.Runtime, pipelineName string, l zerolog.Logger) {
	console := vm.NewObject()

	logFn := func(logger *zerolog.Event, call goja.FunctionCall) goja.Value {
		var b strings.Builder
		for i, arg := range call.Arguments {
			b.WriteString(arg.String())
			if i < len(call.Arguments)-1 {
				b.WriteString("\n")
			}
		}
		logger.Str("pipeline", pipelineName).Msg(b.String())
		return goja.Undefined()
	}

	console.Set("dump", func(call goja.FunctionCall) goja.Value {
		var b strings.Builder
		for _, arg := range call.Arguments {
			obj, err := json.MarshalIndent(arg, "", "  ")
			if err != nil {
				obj = []byte(fmt.Sprintf("console.dump error: %v", err))
			}
			b.WriteString(string(obj))
		}
		// TODO dump should have the same log level as console.log. but it would be nice to differentiate the format level
		l.Info().Str("pipeline", pipelineName).Msg(b.String())
		return goja.Undefined()
	})
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

func (env *JsEnv) addClosable(resource Closable) {
	env.rm.Lock()
	defer env.rm.Unlock()
	env.resources = append(env.resources, resource)
}

func (env *JsEnv) Close() error {
	env.rm.Lock()
	defer env.rm.Unlock()
	var errs []error
	env.loop.Stop()
	for _, resource := range env.resources {
		err := resource.Close()
		errs = append(errs, err)
	}
	env.resources = nil

	return errors.Join(errs...)
}
