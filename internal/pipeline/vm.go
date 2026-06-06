package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"craeft/internal/log"
	"craeft/internal/pipeline/js"
	"craeft/internal/utils"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	"github.com/dop251/goja_nodejs/buffer"
	"github.com/dop251/goja_nodejs/url"
	"github.com/rs/zerolog"
	"golang.org/x/term"
)

// compileBundle parses and compiles a JS bundle string into a goja.Program.
func compileBundle(name, src string) (*goja.Program, error) {
	prg, err := goja.Compile(name, src, false)
	if err == nil {
		return prg, nil
	}

	var ex *goja.Exception
	if errors.As(err, &ex) {
		return nil, fmt.Errorf("js exception compiling %s: %s", name, ex.String())
	}
	var pe *parser.Error
	if errors.As(err, &pe) {
		return nil, fmt.Errorf("js syntax error in %s at %s:%d:%d: %s",
			name, pe.Position.Filename, pe.Position.Line, pe.Position.Column, pe.Message)
	}
	var cref *goja.CompilerReferenceError
	if errors.As(err, &cref) {
		return nil, fmt.Errorf("js reference error in %s: %s", name, cref.Error())
	}
	return nil, fmt.Errorf("js compile error in %s: %w", name, err)
}

type vmConfig struct {
	name   string
	root   *utils.RootFS
	dbPath string

	httpPolicy    js.HTTPPolicy
	httpTransport *http.Transport
}

// vmWorker is a single-use goja runtime.
type vmWorker struct {
	vm     *goja.Runtime
	fn     goja.Callable
	cfg    vmConfig
	logger zerolog.Logger
	ctx    context.Context
}

// newVMWorker creates a fresh goja runtime, runs the pre-compiled program,
// The caller must call close() when done.
func newVMWorker(ctx context.Context, cfg vmConfig, prg *goja.Program) (*vmWorker, error) {
	vm := goja.New()
	l := newConsoleLogger(cfg.name)
	worker := &vmWorker{vm: vm, cfg: cfg, logger: l, ctx: ctx}

	setupVM(worker)

	if _, err := vm.RunProgram(prg); err != nil {
		return nil, fmt.Errorf("run bundle: %w", err)
	}

	pipe := vm.Get(pipeGlobalName)
	if pipe == nil || goja.IsUndefined(pipe) || goja.IsNull(pipe) {
		return nil, fmt.Errorf("pipe %q: bundle did not set global %q", cfg.name, pipeGlobalName)
	}
	v := pipe.ToObject(vm).Get("transform")
	if v == nil || goja.IsUndefined(v) {
		return nil, fmt.Errorf("pipe %q: bundle does not export a transform function", cfg.name)
	}
	fn, ok := goja.AssertFunction(v)
	if !ok {
		return nil, fmt.Errorf("pipe %q: transform is not a function", cfg.name)
	}
	worker.fn = fn

	return worker, nil
}

func (w *vmWorker) transform(ctx context.Context, input []byte, vars map[string]any) ([]byte, error) {
	stop := w.watchContext(ctx)
	defer stop()

	jsVars := w.vm.ToValue(vars)
	jsInput := buffer.WrapBytes(w.vm, input)

	result, err := w.fn(jsVars, jsInput)
	if err != nil {
		var ie *goja.InterruptedError
		if errors.As(err, &ie) && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("pipe %q: transform: %w", w.cfg.name, err)
	}

	switch v := result.Export().(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	case nil:
		return nil, fmt.Errorf("pipe %q: transform returned null/undefined", w.cfg.name)
	default:
		return []byte(result.String()), nil
	}
}

// watchContext spawns a goroutine that interrupts the VM if ctx is cancelled.
// The returned stop func must always be deferred — it clears the interrupt
// and terminates the watcher goroutine.
func (w *vmWorker) watchContext(ctx context.Context) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			w.vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()
	return func() {
		close(done)
		w.vm.ClearInterrupt()
	}
}

func (w *vmWorker) close() {
	// goja has no Close() method — dropping the reference is sufficient.
	w.vm = nil
	w.fn = nil
}

func newConsoleLogger(pipeName string) zerolog.Logger {
	var writer io.Writer = &log.LevelWriter{Writer: os.Stdout, ErrorWriter: os.Stderr}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		cw := zerolog.ConsoleWriter{
			Out:           os.Stdout,
			FieldsExclude: []string{"pipe"},
			PartsOrder:    []string{"level", "pipe", "message"},
		}
		cw.FormatLevel = func(i any) string {
			switch i {
			case "debug":
				return "[console.dump]"
			case "info":
				return "[console.log]"
			case "warn":
				return "[console.warn]"
			case "error":
				return "[console.error]"
			default:
				return fmt.Sprintf("[%s]", i)
			}
		}
		cw.FormatFieldValue = func(i any) string {
			return fmt.Sprintf("pipe:%s:", i)
		}
		cw.FormatMessage = func(i any) string {
			if i == nil || i == "" {
				return ""
			}
			lines := strings.Split(fmt.Sprintf("%s", i), "\n")
			var b strings.Builder
			b.WriteByte('\n')
			for _, line := range lines {
				b.WriteString("> ")
				b.WriteString(line)
				b.WriteByte('\n')
			}
			return b.String()
		}
		writer = cw
	}

	return zerolog.New(writer).With().Timestamp().Str("pipe", pipeName).Logger()
}

func setupGlobals(vm *goja.Runtime) {
	bufMod := vm.NewObject()
	_ = bufMod.Set("exports", vm.NewObject())
	buffer.Require(vm, bufMod)
	bufExports := bufMod.Get("exports").ToObject(vm)
	vm.Set("Buffer", bufExports.Get("Buffer"))

	urlMod := vm.NewObject()
	_ = urlMod.Set("exports", vm.NewObject())
	url.Require(vm, urlMod)
	urlExports := urlMod.Get("exports").ToObject(vm)
	vm.Set("URL", urlExports.Get("URL"))
	vm.Set("URLSearchParams", urlExports.Get("URLSearchParams"))
}

func setupVM(w *vmWorker) {
	w.vm.SetFieldNameMapper(goja.UncapFieldNameMapper())
	setupConsole(w.vm, w.cfg.name, w.logger)
	setupRequire(w)
	// setupCommonJS(vm)
	setupGlobals(w.vm)
}

func setupCommonJS(vm *goja.Runtime) {
	exports := vm.NewObject()
	module := vm.NewObject()
	_ = module.Set("exports", exports)
	vm.Set("exports", exports)
	vm.Set("module", module)
}

func setupRequire(w *vmWorker) {
	w.vm.Set("require", func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		switch name {
		// case "kv":
		// 	kvAPI, err := NewKVStore(cfg.dbPath)
		// 	if err != nil {
		// 		panic(vm.NewGoError(fmt.Errorf("require('kv'): %w", err)))
		// 	}
		// 	return kvAPI.GetModule(vm)
		case "fs":
			fsAPI := js.NewFsApi(w.cfg.root)
			return fsAPI.GetModule(w.vm)
		case "http":
			return js.GetHTTPModule(w.vm, w.ctx, w.cfg.httpPolicy, w.cfg.httpTransport)
		case "md":
			return js.GetMdModule(w.vm)
		case "fasttpl":
			return js.GetFastTplModule(w.vm)
		case "tpl":
			return js.GetTplModule(w.vm)
		}
		// Unknown module — esbuild should have bundled it.
		// Returning undefined lets scripts fail naturally with a clear JS error.
		l := log.Logger().With().Str("pipe", w.cfg.name).Logger()
		l.Warn().Str("module", name).Msg("require: unknown module")
		return goja.Undefined()
	})
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
func setupConsole(vm *goja.Runtime, pipeName string, l zerolog.Logger) {
	logFn := func(logger *zerolog.Event, call goja.FunctionCall) goja.Value {
		var b strings.Builder
		for i, arg := range call.Arguments {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(arg.String())
		}
		logger.Str("pipe", pipeName).Msg(b.String())
		return goja.Undefined()
	}

	console := vm.NewObject()
	console.Set("log", func(call goja.FunctionCall) goja.Value { return logFn(l.Info(), call) })
	console.Set("info", func(call goja.FunctionCall) goja.Value { return logFn(l.Info(), call) })
	console.Set("warn", func(call goja.FunctionCall) goja.Value { return logFn(l.Warn(), call) })
	console.Set("error", func(call goja.FunctionCall) goja.Value { return logFn(l.Error(), call) })
	console.Set("dump", func(call goja.FunctionCall) goja.Value {
		var b strings.Builder
		for _, arg := range call.Arguments {
			obj, err := json.MarshalIndent(arg.Export(), "", "  ")
			if err != nil {
				b.WriteString(fmt.Sprintf("<dump error: %v>", err))
			} else {
				b.WriteString(string(obj))
			}
			b.WriteByte('\n')
		}
		l.Info().Str("pipe", pipeName).Msg(b.String())
		return goja.Undefined()
	})

	vm.Set("console", console)
}
