package js

import (
	"context"
	"net/http"

	"craeft/internal/log"
	"craeft/internal/utils"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
	"github.com/dop251/goja_nodejs/url"
)

// RuntimeConfig configures the modules exposed to a VM by SetupRequire.
type RuntimeConfig struct {
	// Name identifies the pipe for logging (e.g. an unknown require() call).
	Name string

	// Root is the fs module's root. require('fs') resolves to undefined if nil.
	Root *utils.RootFS

	HTTPPolicy    HTTPPolicy
	HTTPTransport *http.Transport
}

// SetupGlobals installs the Buffer and URL/URLSearchParams globals that
// pipeline scripts rely on without an explicit require().
func SetupGlobals(vm *goja.Runtime) {
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

// SetupRequire installs a require() function on vm that resolves this
// package's built-in modules (fs, http, md, fasttpl, tpl, yaml). Unknown
// module names resolve to undefined — esbuild should have bundled anything
// else, so scripts fail naturally with a clear JS error.
func SetupRequire(ctx context.Context, vm *goja.Runtime, cfg RuntimeConfig) {
	vm.Set("require", func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		switch name {
		// case "kv":
		// 	kvAPI, err := NewKVStore(cfg.dbPath)
		// 	if err != nil {
		// 		panic(vm.NewGoError(fmt.Errorf("require('kv'): %w", err)))
		// 	}
		// 	return kvAPI.GetModule(vm)
		case "fs":
			if cfg.Root == nil {
				return goja.Undefined()
			}
			return NewFsApi(cfg.Root).GetModule(vm)
		case "http":
			return GetHTTPModule(vm, ctx, cfg.HTTPPolicy, cfg.HTTPTransport)
		case "md":
			return GetMdModule(vm)
		case "fasttpl":
			return GetFastTplModule(vm)
		case "tpl":
			return GetTplModule(vm)
		case "yaml":
			return GetYamlModule(vm)
		}
		l := log.Logger().With().Str("pipe", cfg.Name).Logger()
		l.Warn().Str("module", name).Msg("require: unknown module")
		return goja.Undefined()
	})
}
