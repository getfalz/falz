package api

import (
	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
)

func GetEsbuildModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()

	module.Set("build", fetch(vm))
	module.Set("transform", fetch(vm))
	return module
}

func build(vm *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("build: Expected one argument (options object)"))
		}

		options := parseBuildOptions(vm, call.Arguments[0])
		result := api.Build(options)

		return vm.ToValue(map[string]interface{}{
			"errors":   result.Errors,
			"warnings": result.Warnings,
			"outputFiles": func() []map[string]interface{} {
				outputs := []map[string]interface{}{}
				for _, file := range result.OutputFiles {
					outputs = append(outputs, map[string]interface{}{
						"path":     file.Path,
						"contents": string(file.Contents),
					})
				}
				return outputs
			}(),
		})
	}
}

func transform(vm *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("transform: Expected two arguments (source string, options object)"))
		}

		promise, resolve, reject := vm.NewPromise()

		go func() {
			source, err := getStringArg(call.Arguments[0])
			if err != nil {
				reject(err)
				return
			}
			options := parseTransformOptions(vm, call.Arguments[1])

			result := api.Transform(source, options)

			obj := vm.ToValue(map[string]interface{}{
				"code":     result.Code,
				"map":      result.Map,
				"errors":   result.Errors,
				"warnings": result.Warnings,
			})

			resolve(obj)
		}()

		return vm.ToValue(promise)

	}
}

func parseBuildOptions(vm *goja.Runtime, val goja.Value) api.BuildOptions {
	obj := val.ToObject(vm)

	options := api.BuildOptions{}
	options.EntryPoints = toStringSlice(obj.Get("entryPoints"))
	options.Outfile = obj.Get("outfile").String()
	options.Bundle = obj.Get("bundle").ToBoolean()
	options.MinifyWhitespace = obj.Get("minifyWhitespace").ToBoolean()
	options.MinifyIdentifiers = obj.Get("minifyIdentifiers").ToBoolean()
	options.MinifySyntax = obj.Get("minifySyntax").ToBoolean()

	// TODO add more fields based on Esbuild's API
	return options
}

func parseTransformOptions(vm *goja.Runtime, val goja.Value) api.TransformOptions {
	obj := val.ToObject(vm)

	options := api.TransformOptions{}
	// https://github.com/evanw/esbuild/blob/main/internal/cli_helpers/cli_helpers.go#L47
	// https://github.com/evanw/esbuild/blob/main/pkg/cli/cli.go#L53
	// https://github.com/evanw/esbuild/blob/main/pkg/cli/cli_impl.go#L84
	// options.Loader = api.LoaderFromString(obj.Get("loader").String())
	options.Sourcefile = obj.Get("sourcefile").String()
	// options.Sourcemap = api.SourceMapFromString(obj.Get("sourcemap").String())
	options.MinifyWhitespace = obj.Get("minifyWhitespace").ToBoolean()
	options.MinifyIdentifiers = obj.Get("minifyIdentifiers").ToBoolean()
	options.MinifySyntax = obj.Get("minifySyntax").ToBoolean()

	// TODO add more fields based on Esbuild's API
	return options
}

func toStringSlice(val goja.Value) []string {
	if val == nil || goja.IsUndefined(val) {
		return nil
	}

	array := val.(*goja.Object)
	length := array.Get("length").ToInteger()
	result := make([]string, length)

	for i := int64(0); i < length; i++ {
		result[i] = array.Get(string(i)).String()
	}

	return result
}
