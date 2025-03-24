package js

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
)

func GetEsbuildModule(ev *EventLoop) *goja.Object {
	module := ev.vm.NewObject()

	module.Set("build", func(call goja.FunctionCall) goja.Value {
		return build(call, ev)
	})
	module.Set("transform", func(call goja.FunctionCall) goja.Value {
		return transform(call, ev)
	})
	return module
}

func build(call goja.FunctionCall, ev *EventLoop) goja.Value {
	promise, resolve, reject := ev.vm.NewPromise()

	arg0 := call.Argument(0)

	go func() {
		obj, err := argMustObject(arg0)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError("build: Expected one argument (options object)"))
			})
			return
		}

		var opts api.BuildOptions
		err = parseOptionsFromGoja(obj, &opts, nil)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewGoError(err))
			})
			return
		}
		result := api.Build(opts)

		resulto := ev.vm.ToValue(map[string]interface{}{
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

		ev.RunOnLoop(func(vm *goja.Runtime) {
			resolve(resulto)
		})

	}()

	return ev.vm.ToValue(promise)

}

func transform(call goja.FunctionCall, ev *EventLoop) goja.Value {
	promise, resolve, reject := ev.vm.NewPromise()

	arg0 := call.Argument(0)
	arg1 := call.Argument(1)

	go func() {
		source, err := argMustString(arg0)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewGoError(err))
			})
			return
		}
		obj, err := argMustObject(arg1)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError("build: Expected one argument (options object)"))
			})
			return
		}

		var opts api.TransformOptions
		err = parseOptionsFromGoja(obj, nil, &opts)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewGoError(err))
			})
			return
		}

		result := api.Transform(source, opts)

		resulto := ev.vm.ToValue(map[string]interface{}{
			"code":     result.Code,
			"map":      result.Map,
			"errors":   result.Errors,
			"warnings": result.Warnings,
		})

		ev.RunOnLoop(func(vm *goja.Runtime) {
			resolve(resulto)
		})
	}()

	return ev.vm.ToValue(promise)
}

// parseOptionsFromGoja converts a Goja options object directly into esbuild options.
// It supports all options (build or transform) by iterating over the Goja object’s keys.
func parseOptionsFromGoja(obj *goja.Object, buildOpts *api.BuildOptions, transformOpts *api.TransformOptions) error {
	// Iterate over each property on the object.
	for _, key := range obj.Keys() {
		v := obj.Get(key)
		if goja.IsUndefined(v) || goja.IsNull(v) {
			continue
		}
		switch key {
		// ––––––– Boolean Options –––––––
		case "bundle":
			if buildOpts != nil {
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for bundle", "Expected a boolean")
				}
				buildOpts.Bundle = b
			}
		case "preserveSymlinks":
			if buildOpts != nil {
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for preserveSymlinks", "Expected a boolean")
				}
				buildOpts.PreserveSymlinks = b
			}
		case "splitting":
			if buildOpts != nil {
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for splitting", "Expected a boolean")
				}
				buildOpts.Splitting = b
			}
		case "allowOverwrite":
			if buildOpts != nil {
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for allowOverwrite", "Expected a boolean")
				}
				buildOpts.AllowOverwrite = b
			}
		case "minify":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for minify", "Expected a boolean")
				}
				if buildOpts != nil {
					buildOpts.MinifySyntax = b
					buildOpts.MinifyWhitespace = b
					buildOpts.MinifyIdentifiers = b
				} else if transformOpts != nil {
					transformOpts.MinifySyntax = b
					transformOpts.MinifyWhitespace = b
					transformOpts.MinifyIdentifiers = b
				}
			}
		case "minifySyntax":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for minifySyntax", "Expected a boolean")
				}
				if buildOpts != nil {
					buildOpts.MinifySyntax = b
				} else if transformOpts != nil {
					transformOpts.MinifySyntax = b
				}
			}
		case "minifyWhitespace":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for minifyWhitespace", "Expected a boolean")
				}
				if buildOpts != nil {
					buildOpts.MinifyWhitespace = b
				} else if transformOpts != nil {
					transformOpts.MinifyWhitespace = b
				}
			}
		case "minifyIdentifiers":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for minifyIdentifiers", "Expected a boolean")
				}
				if buildOpts != nil {
					buildOpts.MinifyIdentifiers = b
				} else if transformOpts != nil {
					transformOpts.MinifyIdentifiers = b
				}
			}
		case "mangleQuoted":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for mangleQuoted", "Expected a boolean")
				}
				var target *api.MangleQuoted
				if buildOpts != nil {
					target = &buildOpts.MangleQuoted
				} else if transformOpts != nil {
					target = &transformOpts.MangleQuoted
				}
				if b {
					*target = api.MangleQuotedTrue
				} else {
					*target = api.MangleQuotedFalse
				}
			}
		case "treeShaking":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for treeShaking", "Expected a boolean")
				}
				if buildOpts != nil {
					buildOpts.TreeShaking = boolToTreeShaking(b)
				} else if transformOpts != nil {
					transformOpts.TreeShaking = boolToTreeShaking(b)
				}
			}
		case "ignoreAnnotations":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for ignoreAnnotations", "Expected a boolean")
				}
				if buildOpts != nil {
					buildOpts.IgnoreAnnotations = b
				} else if transformOpts != nil {
					transformOpts.IgnoreAnnotations = b
				}
			}
		case "keepNames":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for keepNames", "Expected a boolean")
				}
				if buildOpts != nil {
					buildOpts.KeepNames = b
				} else if transformOpts != nil {
					transformOpts.KeepNames = b
				}
			}
		// ––––––– Sourcemap (boolean or string) –––––––
		case "sourcemap":
			{
				// For build mode a bare flag (boolean true) means "linked"
				if buildOpts != nil {
					switch t := v.Export().(type) {
					case bool:
						if t {
							buildOpts.Sourcemap = api.SourceMapLinked
						}
					case string:
						sm, convErr := convertSourceMap(t)
						if convErr != nil {
							return fmt.Errorf(convErr.Error(), "Valid values: linked, inline, external, both")
						}
						buildOpts.Sourcemap = sm
					}
				} else if transformOpts != nil {
					switch t := v.Export().(type) {
					case bool:
						if t {
							transformOpts.Sourcemap = api.SourceMapInline
						}
					case string:
						sm, convErr := convertSourceMap(t)
						if convErr != nil {
							return fmt.Errorf(convErr.Error(), "Valid values: linked, inline, external, both")
						}
						transformOpts.Sourcemap = sm
					}
				}
			}
		// ––––––– String Options –––––––
		case "sourceRoot":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for sourceRoot", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.SourceRoot = s
				} else if transformOpts != nil {
					transformOpts.SourceRoot = s
				}
			}
		// ––––––– Sources Content –––––––
		case "sourcesContent":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for sourcesContent", "Expected a boolean")
				}
				if buildOpts != nil {
					if b {
						buildOpts.SourcesContent = api.SourcesContentInclude
					} else {
						buildOpts.SourcesContent = api.SourcesContentExclude
					}
				} else if transformOpts != nil {
					if b {
						transformOpts.SourcesContent = api.SourcesContentInclude
					} else {
						transformOpts.SourcesContent = api.SourcesContentExclude
					}
				}
			}
		case "sourcefile":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for sourcefile", "Expected a string")
				}
				if buildOpts != nil {
					if buildOpts.Stdin == nil {
						buildOpts.Stdin = &api.StdinOptions{}
					}
					buildOpts.Stdin.Sourcefile = s
				} else if transformOpts != nil {
					transformOpts.Sourcefile = s
				}
			}
		// ––––––– Array (or comma‐separated string) Options –––––––
		case "resolveExtensions", "mainFields", "conditions":
			{
				arr, convErr := getStringArray(v)
				if convErr != nil {
					return fmt.Errorf("Invalid type for "+key, "Expected an array or comma‐separated string")
				}
				switch key {
				case "resolveExtensions":
					if buildOpts != nil {
						buildOpts.ResolveExtensions = arr
					}
				case "mainFields":
					if buildOpts != nil {
						buildOpts.MainFields = arr
					}
				case "conditions":
					if buildOpts != nil {
						buildOpts.Conditions = arr
					}
				}
			}
		case "publicPath":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for publicPath", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.PublicPath = s
				}
			}
		case "globalName":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for globalName", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.GlobalName = s
				} else if transformOpts != nil {
					transformOpts.GlobalName = s
				}
			}
		// ––––––– metafile –––––––
		case "metafile":
			{
				switch t := v.Export().(type) {
				case bool:
					if t && buildOpts != nil {
						buildOpts.Metafile = true
					}
				case string:
				default:
					return fmt.Errorf("Invalid type for metafile", "Expected a boolean or string")
				}
			}
		// ––––––– Output Options –––––––
		case "outfile":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for outfile", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.Outfile = s
				}
			}
		case "outdir":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for outdir", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.Outdir = s
				}
			}
		case "outbase":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for outbase", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.Outbase = s
				}
			}
		case "tsconfig":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for tsconfig", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.Tsconfig = s
				}
			}
		case "tsconfigRaw":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for tsconfigRaw", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.TsconfigRaw = s
				} else if transformOpts != nil {
					transformOpts.TsconfigRaw = s
				}
			}
		case "entryNames":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for entryNames", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.EntryNames = s
				}
			}
		case "chunkNames":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for chunkNames", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.ChunkNames = s
				}
			}
		case "assetNames":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for assetNames", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.AssetNames = s
				}
			}
		// ––––––– Key/Value Object Options –––––––
		case "define":
			{
				m, ok := v.Export().(map[string]interface{})
				if !ok {
					return fmt.Errorf("Invalid type for define", "Expected an object")
				}
				if buildOpts != nil {
					if buildOpts.Define == nil {
						buildOpts.Define = make(map[string]string)
					}
					for k, v2 := range m {
						buildOpts.Define[k] = fmt.Sprintf("%v", v2)
					}
				} else if transformOpts != nil {
					if transformOpts.Define == nil {
						transformOpts.Define = make(map[string]string)
					}
					for k, v2 := range m {
						transformOpts.Define[k] = fmt.Sprintf("%v", v2)
					}
				}
			}
		case "logOverride":
			{
				m, ok := v.Export().(map[string]string)
				if !ok {
					return fmt.Errorf("Invalid type for logOverride", "Expected an object")
				}
				if buildOpts != nil {
					if buildOpts.LogOverride == nil {
						buildOpts.LogOverride = make(map[string]api.LogLevel)
					}
					for k, v2 := range m {
						level, err := parseLogLevel(v2)
						if err != nil {
							return err
						}
						buildOpts.LogOverride[k] = level
					}
				} else if transformOpts != nil {
					if transformOpts.LogOverride == nil {
						transformOpts.LogOverride = make(map[string]api.LogLevel)
					}
					for k, v2 := range m {
						level, err := parseLogLevel(v2)
						if err != nil {
							return err
						}
						transformOpts.LogOverride[k] = level
					}
				}
			}
		case "supported":
			{
				m, ok := v.Export().(map[string]interface{})
				if !ok {
					return fmt.Errorf("Invalid type for supported", "Expected an object")
				}
				if buildOpts != nil {
					if buildOpts.Supported == nil {
						buildOpts.Supported = make(map[string]bool)
					}
					for k, v2 := range m {
						if b, ok := v2.(bool); ok {
							buildOpts.Supported[k] = b
						} else {
							return fmt.Errorf("Invalid value in supported", "Expected boolean values")
						}
					}
				} else if transformOpts != nil {
					if transformOpts.Supported == nil {
						transformOpts.Supported = make(map[string]bool)
					}
					for k, v2 := range m {
						if b, ok := v2.(bool); ok {
							transformOpts.Supported[k] = b
						} else {
							return fmt.Errorf("Invalid value in supported", "Expected boolean values")
						}
					}
				}
			}
		// ––––––– Repeated Options (string or array) –––––––
		case "drop":
			{
				// “drop” may be a string or an array of strings.
				switch t := v.Export().(type) {
				case string:
					if err := setDropOption(t, buildOpts, transformOpts); err != nil {
						return fmt.Errorf(err.Error(), "Valid values are \"console\" or \"debugger\"")
					}
				case []interface{}:
					for _, el := range t {
						s, ok := el.(string)
						if !ok {
							return fmt.Errorf("Invalid type in drop array", "Expected string")
						}
						if err := setDropOption(s, buildOpts, transformOpts); err != nil {
							return fmt.Errorf(err.Error(), "Valid values are \"console\" or \"debugger\"")
						}
					}
				default:
					return fmt.Errorf("Invalid type for drop", "Expected string or array")
				}
			}
		case "dropLabels":
			{
				arr, convErr := getStringArray(v)
				if convErr != nil {
					return fmt.Errorf("Invalid type for dropLabels", "Expected array or comma‐separated string")
				}
				if buildOpts != nil {
					buildOpts.DropLabels = arr
				} else if transformOpts != nil {
					transformOpts.DropLabels = arr
				}
			}
		case "pure":
			{
				switch t := v.Export().(type) {
				case string:
					if buildOpts != nil {
						buildOpts.Pure = append(buildOpts.Pure, t)
					} else if transformOpts != nil {
						transformOpts.Pure = append(transformOpts.Pure, t)
					}
				case []interface{}:
					for _, el := range t {
						s, ok := el.(string)
						if !ok {
							return fmt.Errorf("Invalid type in pure array", "Expected string")
						}
						if buildOpts != nil {
							buildOpts.Pure = append(buildOpts.Pure, s)
						} else if transformOpts != nil {
							transformOpts.Pure = append(transformOpts.Pure, s)
						}
					}
				default:
					return fmt.Errorf("Invalid type for pure", "Expected string or array")
				}
			}
		// ––––––– Loader –––––––
		case "loader":
			{
				// In build mode, loader can be an object mapping file extension to loader string.
				if buildOpts != nil {
					switch t := v.Export().(type) {
					case string:
						if err := setLoaderForBuild("", t, buildOpts); err != nil {
							return fmt.Errorf(err.Error(), "Invalid loader value")
						}
					case map[string]interface{}:
						for k, v2 := range t {
							s := fmt.Sprintf("%v", v2)
							if err := setLoaderForBuild(k, s, buildOpts); err != nil {
								return fmt.Errorf(err.Error(), "Invalid loader value")
							}
						}
					default:
						return fmt.Errorf("Invalid type for loader", "Expected string or object")
					}
				} else if transformOpts != nil {
					s, ok := v.Export().(string)
					if !ok {
						return fmt.Errorf("Invalid type for loader", "Expected string when transforming")
					}
					loader, errL := parseLoader(s)
					if errL != nil {
						return errL
					}
					transformOpts.Loader = loader
				}
			}
		// ––––––– Target –––––––
		case "target":
			{
				arr, convErr := getStringArray(v)
				if convErr != nil {
					return fmt.Errorf("Invalid type for target", "Expected array or comma‐separated string")
				}
				if buildOpts != nil {
					t, engines, errT := parseTargets(arr, "target")
					if errT != nil {
						return errT
					}
					buildOpts.Target = t
					buildOpts.Engines = engines
				} else if transformOpts != nil {
					t, engines, errT := parseTargets(arr, "target")
					if errT != nil {
						return errT
					}
					transformOpts.Target = t
					transformOpts.Engines = engines
				}
			}
		case "outExtension":
			{
				m, ok := v.Export().(map[string]interface{})
				if !ok {
					return fmt.Errorf("Invalid type for outExtension", "Expected object")
				}
				if buildOpts != nil {
					if buildOpts.OutExtension == nil {
						buildOpts.OutExtension = make(map[string]string)
					}
					for k, v2 := range m {
						buildOpts.OutExtension[k] = fmt.Sprintf("%v", v2)
					}
				}
			}
		// ––––––– Platform, Format, Packages –––––––
		case "platform":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for platform", "Expected string")
				}
				if buildOpts != nil {
					buildOpts.Platform = convertPlatform(s)
				} else if transformOpts != nil {
					transformOpts.Platform = convertPlatform(s)
				}
			}
		case "format":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for format", "Expected string")
				}
				if buildOpts != nil {
					buildOpts.Format = convertFormat(s)
				} else if transformOpts != nil {
					transformOpts.Format = convertFormat(s)
				}
			}
		case "packages":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for packages", "Expected string")
				}
				if buildOpts != nil {
					buildOpts.Packages = convertPackages(s)
				}
			}
		// ––––––– Repeated Array Options –––––––
		case "external":
			{
				arr, convErr := getStringArray(v)
				if convErr != nil {
					return fmt.Errorf("Invalid type for external", "Expected array or string")
				}
				if buildOpts != nil {
					buildOpts.External = append(buildOpts.External, arr...)
				}
			}
		case "inject":
			{
				arr, convErr := getStringArray(v)
				if convErr != nil {
					return fmt.Errorf("Invalid type for inject", "Expected array or string")
				}
				if buildOpts != nil {
					buildOpts.Inject = append(buildOpts.Inject, arr...)
				}
			}
		case "alias":
			{
				m, ok := v.Export().(map[string]interface{})
				if !ok {
					return fmt.Errorf("Invalid type for alias", "Expected object")
				}
				if buildOpts != nil {
					if buildOpts.Alias == nil {
						buildOpts.Alias = make(map[string]string)
					}
					for k, v2 := range m {
						buildOpts.Alias[k] = fmt.Sprintf("%v", v2)
					}
				}
			}
		// ––––––– JSX Options –––––––
		case "jsx":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for jsx", "Expected string")
				}
				if buildOpts != nil {
					buildOpts.JSX = convertJSX(s)
				} else if transformOpts != nil {
					transformOpts.JSX = convertJSX(s)
				}
			}
		case "jsxFactory":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for jsxFactory", "Expected string")
				}
				if buildOpts != nil {
					buildOpts.JSXFactory = s
				} else if transformOpts != nil {
					transformOpts.JSXFactory = s
				}
			}
		case "jsxFragment":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for jsxFragment", "Expected string")
				}
				if buildOpts != nil {
					buildOpts.JSXFragment = s
				} else if transformOpts != nil {
					transformOpts.JSXFragment = s
				}
			}
		case "jsxImportSource":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for jsxImportSource", "Expected string")
				}
				if buildOpts != nil {
					buildOpts.JSXImportSource = s
				} else if transformOpts != nil {
					transformOpts.JSXImportSource = s
				}
			}
		case "jsxDev":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for jsxDev", "Expected boolean")
				}
				if buildOpts != nil {
					buildOpts.JSXDev = b
				} else if transformOpts != nil {
					transformOpts.JSXDev = b
				}
			}
		case "jsxSideEffects":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for jsxSideEffects", "Expected boolean")
				}
				if buildOpts != nil {
					buildOpts.JSXSideEffects = b
				} else if transformOpts != nil {
					transformOpts.JSXSideEffects = b
				}
			}
		// ––––––– Banner and Footer –––––––
		case "banner":
			{
				// For build mode, banner may be an object mapping language to value.
				if buildOpts != nil {
					switch t := v.Export().(type) {
					case string:
						if buildOpts.Banner == nil {
							buildOpts.Banner = make(map[string]string)
						}
						// Use an empty key as a fallback
						buildOpts.Banner[""] = t
					case map[string]interface{}:
						if buildOpts.Banner == nil {
							buildOpts.Banner = make(map[string]string)
						}
						for k, v2 := range t {
							buildOpts.Banner[k] = fmt.Sprintf("%v", v2)
						}
					default:
						return fmt.Errorf("Invalid type for banner", "Expected string or object")
					}
				} else if transformOpts != nil {
					s, ok := v.Export().(string)
					if !ok {
						return fmt.Errorf("Invalid type for banner", "Expected string")
					}
					transformOpts.Banner = s
				}
			}
		case "footer":
			{
				if buildOpts != nil {
					switch t := v.Export().(type) {
					case string:
						if buildOpts.Footer == nil {
							buildOpts.Footer = make(map[string]string)
						}
						buildOpts.Footer[""] = t
					case map[string]interface{}:
						if buildOpts.Footer == nil {
							buildOpts.Footer = make(map[string]string)
						}
						for k, v2 := range t {
							buildOpts.Footer[k] = fmt.Sprintf("%v", v2)
						}
					default:
						return fmt.Errorf("Invalid type for footer", "Expected string or object")
					}
				} else if transformOpts != nil {
					s, ok := v.Export().(string)
					if !ok {
						return fmt.Errorf("Invalid type for footer", "Expected string")
					}
					transformOpts.Footer = s
				}
			}
		// ––––––– Numeric Options –––––––
		case "logLimit":
			{
				n, errNum := getInt(v)
				if errNum != nil {
					return fmt.Errorf("Invalid type for logLimit", "Expected an integer")
				}
				if buildOpts != nil {
					buildOpts.LogLimit = n
				} else if transformOpts != nil {
					transformOpts.LogLimit = n
				}
			}
		case "lineLimit":
			{
				n, errNum := getInt(v)
				if errNum != nil {
					return fmt.Errorf("Invalid type for lineLimit", "Expected an integer")
				}
				if buildOpts != nil {
					buildOpts.LineLimit = n
				} else if transformOpts != nil {
					transformOpts.LineLimit = n
				}
			}
		// ––––––– Final Options –––––––
		case "color":
			{
				b, ok := v.Export().(bool)
				if !ok {
					return fmt.Errorf("Invalid type for color", "Expected a boolean")
				}
				if buildOpts != nil {
					if b {
						buildOpts.Color = api.ColorAlways
					} else {
						buildOpts.Color = api.ColorNever
					}
				} else if transformOpts != nil {
					if b {
						transformOpts.Color = api.ColorAlways
					} else {
						transformOpts.Color = api.ColorNever
					}
				}
			}
		case "logLevel":
			{
				s, ok := v.Export().(string)
				if !ok {
					return fmt.Errorf("Invalid type for logLevel", "Expected a string")
				}
				if buildOpts != nil {
					buildOpts.LogLevel = convertLogLevel(s)
				} else if transformOpts != nil {
					transformOpts.LogLevel = convertLogLevel(s)
				}
			}
		// Unknown keys are ignored.
		default:
			// Optionally, log or warn about unrecognized option keys.
		}
	}

	// Special handling: if building and a bare sourcemap flag was set (and no outfile/outdir),
	// change sourcemap to inline.
	if buildOpts != nil && buildOpts.Sourcemap == api.SourceMapLinked && buildOpts.Outfile == "" && buildOpts.Outdir == "" {
		buildOpts.Sourcemap = api.SourceMapInline
	}

	return nil
}

// getStringArray extracts a []string from a Goja value that can be either an array or a comma‐separated string.
func getStringArray(v goja.Value) ([]string, error) {
	switch t := v.Export().(type) {
	case []interface{}:
		var arr []string
		for _, item := range t {
			arr = append(arr, fmt.Sprintf("%v", item))
		}
		return arr, nil
	case string:
		// Split by comma and trim spaces.
		parts := strings.Split(t, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}
		return parts, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to []string", t)
	}
}

func parseLogLevel(value string) (api.LogLevel, error) {
	switch value {
	case "verbose":
		return api.LogLevelVerbose, nil
	case "debug":
		return api.LogLevelDebug, nil
	case "info":
		return api.LogLevelInfo, nil
	case "warning":
		return api.LogLevelWarning, nil
	case "error":
		return api.LogLevelError, nil
	case "silent":
		return api.LogLevelSilent, nil
	default:
		return api.LogLevelSilent, fmt.Errorf(
			fmt.Sprintf("Invalid value %q", value),
			"Valid values are \"verbose\", \"debug\", \"info\", \"warning\", \"error\", or \"silent\".",
		)
	}
}

// getInt converts a Goja value into an int.
func getInt(v goja.Value) (int, error) {
	switch t := v.Export().(type) {
	case int:
		return t, nil
	case float64:
		return int(t), nil
	case string:
		return strconv.Atoi(t)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", t)
	}
}

// boolToTreeShaking converts a boolean to the appropriate esbuild TreeShaking value.
func boolToTreeShaking(b bool) api.TreeShaking {
	if b {
		return api.TreeShakingTrue
	}
	return api.TreeShakingFalse
}

// convertSourceMap converts a string (e.g. "inline", "linked", etc.) to an esbuild SourceMap.
func convertSourceMap(s string) (api.SourceMap, error) {
	switch s {
	case "linked":
		return api.SourceMapLinked, nil
	case "inline":
		return api.SourceMapInline, nil
	case "external":
		return api.SourceMapExternal, nil
	case "both":
		return api.SourceMapInlineAndExternal, nil
	default:
		return api.SourceMapNone, fmt.Errorf("invalid sourcemap value %q", s)
	}
}

// convertPlatform converts a string to an esbuild Platform.
func convertPlatform(s string) api.Platform {
	switch s {
	case "browser":
		return api.PlatformBrowser
	case "node":
		return api.PlatformNode
	case "neutral":
		return api.PlatformNeutral
	default:
		return api.PlatformNeutral
	}
}

// convertFormat converts a string to an esbuild Format.
func convertFormat(s string) api.Format {
	switch s {
	case "iife":
		return api.FormatIIFE
	case "cjs":
		return api.FormatCommonJS
	case "esm":
		return api.FormatESModule
	default:
		return api.FormatESModule
	}
}

// convertPackages converts a string to an esbuild Packages value.
func convertPackages(s string) api.Packages {
	switch s {
	case "bundle":
		return api.PackagesBundle
	case "external":
		return api.PackagesExternal
	default:
		return api.PackagesBundle
	}
}

// setDropOption sets the drop flag based on a given string.
func setDropOption(s string, buildOpts *api.BuildOptions, transformOpts *api.TransformOptions) error {
	switch s {
	case "console":
		if buildOpts != nil {
			buildOpts.Drop |= api.DropConsole
		} else {
			transformOpts.Drop |= api.DropConsole
		}
	case "debugger":
		if buildOpts != nil {
			buildOpts.Drop |= api.DropDebugger
		} else {
			transformOpts.Drop |= api.DropDebugger
		}
	default:
		return fmt.Errorf("invalid drop option %q", s)
	}
	return nil
}

// setLoaderForBuild parses a loader string and sets it on the BuildOptions for the given extension.
func setLoaderForBuild(ext, s string, buildOpts *api.BuildOptions) error {
	loader, err := parseLoader(s)
	if err != nil {
		return err
	}
	if buildOpts.Loader == nil {
		buildOpts.Loader = make(map[string]api.Loader)
	}
	// If ext is empty, assume stdin
	if ext == "" {
		if buildOpts.Stdin == nil {
			buildOpts.Stdin = &api.StdinOptions{}
		}
		buildOpts.Stdin.Loader = loader
	} else {
		buildOpts.Loader[ext] = loader
	}
	return nil
}

// convertJSX converts a string to an esbuild JSX mode.
func convertJSX(s string) api.JSX {
	switch s {
	case "transform":
		return api.JSXTransform
	case "preserve":
		return api.JSXPreserve
	case "automatic":
		return api.JSXAutomatic
	default:
		return api.JSXTransform
	}
}

// convertLogLevel converts a string to an esbuild LogLevel.
func convertLogLevel(s string) api.LogLevel {
	switch s {
	case "verbose":
		return api.LogLevelVerbose
	case "debug":
		return api.LogLevelDebug
	case "info":
		return api.LogLevelInfo
	case "warning":
		return api.LogLevelWarning
	case "error":
		return api.LogLevelError
	default:
		return api.LogLevelInfo
	}
}

func parseLoader(text string) (api.Loader, error) {
	switch text {
	case "base64":
		return api.LoaderBase64, nil
	case "binary":
		return api.LoaderBinary, nil
	case "copy":
		return api.LoaderCopy, nil
	case "css":
		return api.LoaderCSS, nil
	case "dataurl":
		return api.LoaderDataURL, nil
	case "default":
		return api.LoaderDefault, nil
	case "empty":
		return api.LoaderEmpty, nil
	case "file":
		return api.LoaderFile, nil
	case "global-css":
		return api.LoaderGlobalCSS, nil
	case "js":
		return api.LoaderJS, nil
	case "json":
		return api.LoaderJSON, nil
	case "jsx":
		return api.LoaderJSX, nil
	case "local-css":
		return api.LoaderLocalCSS, nil
	case "text":
		return api.LoaderText, nil
	case "ts":
		return api.LoaderTS, nil
	case "tsx":
		return api.LoaderTSX, nil
	default:
		return api.LoaderNone, fmt.Errorf(
			"Invalid loader value: %q", text,
			"Valid values are \"base64\", \"binary\", \"copy\", \"css\", \"dataurl\", \"empty\", \"file\", \"global-css\", \"js\", \"json\", \"jsx\", \"local-css\", \"text\", \"ts\", or \"tsx\".",
		)
	}
}

// TODO do we need all engines? code only runs in goja. what would be a adequate engine? or remove the whole option
var validEngines = map[string]api.EngineName{
	"chrome":  api.EngineChrome,
	"deno":    api.EngineDeno,
	"edge":    api.EngineEdge,
	"firefox": api.EngineFirefox,
	"hermes":  api.EngineHermes,
	"ie":      api.EngineIE,
	"ios":     api.EngineIOS,
	"node":    api.EngineNode,
	"opera":   api.EngineOpera,
	"rhino":   api.EngineRhino,
	"safari":  api.EngineSafari,
}

func parseTargets(targets []string, arg string) (target api.Target, engines []api.Engine, err error) {
	validTargets := map[string]api.Target{
		"esnext": api.ESNext,
		"es5":    api.ES5,
		"es6":    api.ES2015,
		"es2015": api.ES2015,
		"es2016": api.ES2016,
		"es2017": api.ES2017,
		"es2018": api.ES2018,
		"es2019": api.ES2019,
		"es2020": api.ES2020,
		"es2021": api.ES2021,
		"es2022": api.ES2022,
		"es2023": api.ES2023,
		"es2024": api.ES2024,
	}

outer:
	for _, value := range targets {
		if valid, ok := validTargets[strings.ToLower(value)]; ok {
			target = valid
			continue
		}

		for engine, name := range validEngines {
			if strings.HasPrefix(value, engine) {
				version := value[len(engine):]
				if version == "" {
					return 0, nil, fmt.Errorf(
						"Target %q is missing a version number in %q", value, arg,
					)
				}
				engines = append(engines, api.Engine{Name: name, Version: version})
				continue outer
			}
		}

		engines := make([]string, 0, len(validEngines))
		engines = append(engines, "\"esN\"")
		for key := range validEngines {
			engines = append(engines, fmt.Sprintf("%q", key+"N"))
		}
		sort.Strings(engines)
		return 0, nil, fmt.Errorf(
			fmt.Sprintf("Invalid target %q in %q", value, arg),
			fmt.Sprintf("Valid values are %s, or %s where N is a version number.",
				strings.Join(engines[:len(engines)-1], ", "), engines[len(engines)-1]),
		)
	}
	return
}
