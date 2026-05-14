package js

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/gobuffalo/plush"
)

// @ts namespace tpl
//
// @doc
// A utility module for rendering and manipulating templates using the Plush system.
//
// This module provides various functions that support rendering templates,
// working with variables, and performing other templating tasks.
// It uses the Plush template engine from Go (`github.com/gobuffalo/plush`).
//
// Functions available in this module include `render`, which renders a template string
// using the provided variables, and other utilities that may support different use cases.
func GetTplModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("render", func(call goja.FunctionCall) goja.Value { return tplRender(vm, call) })
	return module
}

// @ts function tpl.render
//
// @doc
// Renders a Plush template string against a variables map and returns the result.
// Plush supports ERB-style tags:
//   - <%= expr %>  outputs the result of expr (HTML-escaped by default)
//   - <%== expr %> outputs the result of expr without escaping
//   - <% stmt %>   executes a statement (if, for, etc.) without output
//
// Throws a TypeError if arguments have the wrong type.
// Throws an Error if template rendering fails (syntax error, undefined variable, etc.).
//
// @param template - string - A template string with embedded dynamic content (e.g., "<%= name %>").
// @param variables - Record<string, any> - An object (map) containing key-value pairs to replace placeholders in the template.
// @returns string - A promise that resolves to the rendered string with variables replaced.
func tplRender(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	tpl, err := argMustString(call.Argument(0))
	if err != nil {
		panic(vm.NewTypeError("tpl.render: first argument must be a string: %v", err))
	}

	var vars map[string]any
	if err := vm.ExportTo(call.Argument(1), &vars); err != nil {
		panic(vm.NewTypeError("tpl.render: second argument must be an object: %v", err))
	}

	result, err := plush.Render(tpl, plush.NewContextWith(vars))
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("tpl.render: %w", err)))
	}

	return vm.ToValue(result)
}
