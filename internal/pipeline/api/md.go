package api

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type MdApi struct {
	parser   *parser.Parser
	renderer *html.Renderer
}

func GetMdModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("Parser", NewMdApiConstructor(vm))
	return module
}

func NewMdApiConstructor(vm *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("MdApi requires a configuration object"))
		}

		optionsArg := call.Argument(0).ToObject(vm)
		config := MdApiConfig{
			Parser:   make(map[string]interface{}),
			Renderer: make(map[string]interface{}),
		}

		if parserConfig := optionsArg.Get("parser"); parserConfig != nil {
			if parserObj, ok := parserConfig.ToObject(vm).Export().(map[string]interface{}); ok {
				config.Parser = parserObj
			}
		}

		if rendererConfig := optionsArg.Get("renderer"); rendererConfig != nil {
			if rendererObj, ok := rendererConfig.ToObject(vm).Export().(map[string]interface{}); ok {
				config.Renderer = rendererObj
			}
		}

		mdApi := NewMdApi(vm, config)

		return vm.ToValue(mdApi).ToObject(vm)
	}
}

type MdApiConfig struct {
	Parser   map[string]interface{}
	Renderer map[string]interface{}
}

func NewMdApi(vm *goja.Runtime, options MdApiConfig) *MdApi {
	parserExtensions := parser.CommonExtensions
	rendererFlags := html.CommonFlags

	for key, value := range options.Parser {
		if enabled, ok := value.(bool); ok && enabled {
			switch key {
			case "NoIntraEmphasis":
				parserExtensions |= parser.NoIntraEmphasis
			case "Tables":
				parserExtensions |= parser.Tables
			case "FencedCode":
				parserExtensions |= parser.FencedCode
			case "Autolink":
				parserExtensions |= parser.Autolink
			case "Strikethrough":
				parserExtensions |= parser.Strikethrough
			case "LaxHTMLBlocks":
				parserExtensions |= parser.LaxHTMLBlocks
			case "SpaceHeadings":
				parserExtensions |= parser.SpaceHeadings
			case "HardLineBreak":
				parserExtensions |= parser.HardLineBreak
			case "NonBlockingSpace":
				parserExtensions |= parser.NonBlockingSpace
			case "TabSizeEight":
				parserExtensions |= parser.TabSizeEight
			case "Footnotes":
				parserExtensions |= parser.Footnotes
			case "NoEmptyLineBeforeBlock":
				parserExtensions |= parser.NoEmptyLineBeforeBlock
			case "HeadingIDs":
				parserExtensions |= parser.HeadingIDs
			case "Titleblock":
				parserExtensions |= parser.Titleblock
			case "AutoHeadingIDs":
				parserExtensions |= parser.AutoHeadingIDs
			case "BackslashLineBreak":
				parserExtensions |= parser.BackslashLineBreak
			case "DefinitionLists":
				parserExtensions |= parser.DefinitionLists
			case "MathJax":
				parserExtensions |= parser.MathJax
			case "OrderedListStart":
				parserExtensions |= parser.OrderedListStart
			case "Attributes":
				parserExtensions |= parser.Attributes
			case "SuperSubscript":
				parserExtensions |= parser.SuperSubscript
			case "EmptyLinesBreakList":
				parserExtensions |= parser.EmptyLinesBreakList
			case "Includes":
				parserExtensions |= parser.Includes
			case "Mmark":
				parserExtensions |= parser.Mmark
			}
		}
	}

	for key, value := range options.Renderer {
		if enabled, ok := value.(bool); ok && enabled {
			switch key {
			case "HrefTargetBlank":
				rendererFlags |= html.HrefTargetBlank
			case "SkipHTML":
				rendererFlags |= html.SkipHTML
			case "SkipImages":
				rendererFlags |= html.SkipImages
			case "SkipLinks":
				rendererFlags |= html.SkipLinks
			case "Safelink":
				rendererFlags |= html.Safelink
			case "NofollowLinks":
				rendererFlags |= html.NofollowLinks
			case "NoreferrerLinks":
				rendererFlags |= html.NoreferrerLinks
			case "NoopenerLinks":
				rendererFlags |= html.NoopenerLinks
			case "CompletePage":
				rendererFlags |= html.CompletePage
			case "UseXHTML":
				rendererFlags |= html.UseXHTML
			case "FootnoteReturnLinks":
				rendererFlags |= html.FootnoteReturnLinks
			case "FootnoteNoHRTag":
				rendererFlags |= html.FootnoteNoHRTag
			case "Smartypants":
				rendererFlags |= html.Smartypants
			case "SmartypantsFractions":
				rendererFlags |= html.SmartypantsFractions
			case "SmartypantsDashes":
				rendererFlags |= html.SmartypantsDashes
			case "SmartypantsLatexDashes":
				rendererFlags |= html.SmartypantsLatexDashes
			case "SmartypantsAngledQuotes":
				rendererFlags |= html.SmartypantsAngledQuotes
			case "SmartypantsQuotesNBSP":
				rendererFlags |= html.SmartypantsQuotesNBSP
			case "TOC":
				rendererFlags |= html.TOC
			case "LazyLoadImages":
				rendererFlags |= html.LazyLoadImages
			}
		}
	}

	p := parser.NewWithExtensions(parserExtensions)
	opts := html.RendererOptions{Flags: rendererFlags}
	renderer := html.NewRenderer(opts)

	return &MdApi{
		parser:   p,
		renderer: renderer,
	}
}

func (m *MdApi) Parse(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		if len(call.Arguments) < 1 {
			reject(fmt.Errorf("Parse requires a data argument"))
			return
		}

		arg := call.Arguments[0]

		data, err := m.getByteArg(vm, arg)
		if err != nil {
			reject(err)
			return
		}

		doc := m.parser.Parse(data)
		html := markdown.Render(doc, m.renderer)

		resolve(vm.ToValue(string(html)))
	}()

	return vm.ToValue(promise)
}

func (m *MdApi) getByteArg(vm *goja.Runtime, arg goja.Value) ([]byte, error) {
	if arg == nil || goja.IsUndefined(arg) {
		return nil, fmt.Errorf("data argument is required")
	}

	arrayBufferConstructor := vm.Get("ArrayBuffer").ToObject(vm)
	if arrayBufferConstructor != nil {
		if vm.InstanceOf(arg, arrayBufferConstructor) {
			buffer, ok := arg.Export().(goja.ArrayBuffer)
			if !ok {
				return nil, fmt.Errorf("failed to convert ArrayBuffer")
			}
			return buffer.Bytes(), nil
		}
	}

	uint8ArrayConstructor := vm.Get("Uint8Array").ToObject(vm)
	if uint8ArrayConstructor != nil {
		if vm.InstanceOf(arg, uint8ArrayConstructor) {
			obj := arg.ToObject(vm)

			bufferProp := obj.Get("buffer")
			bufferObj := bufferProp.ToObject(vm)
			buffer, ok := bufferObj.Export().(goja.ArrayBuffer)
			if !ok {
				return nil, fmt.Errorf("failed to retrieve ArrayBuffer from Uint8Array")
			}

			byteOffsetProp := obj.Get("byteOffset")
			byteLengthProp := obj.Get("byteLength")
			byteOffset := int(byteOffsetProp.ToInteger())
			byteLength := int(byteLengthProp.ToInteger())

			bufferBytes := buffer.Bytes()
			if byteOffset < 0 || byteLength < 0 || byteOffset+byteLength > len(bufferBytes) {
				return nil, fmt.Errorf("invalid byteOffset or byteLength")
			}

			data := bufferBytes[byteOffset : byteOffset+byteLength]
			return data, nil
		}
	}

	if str, ok := arg.Export().(string); ok {
		return []byte(str), nil
	}

	return nil, fmt.Errorf("data must be a Uint8Array, ArrayBuffer, or string")
}

func (m *MdApi) Get(key string) goja.Value {
	return nil
}

func (m *MdApi) Set(key string, val goja.Value) bool {
	return false
}

func (m *MdApi) Has(key string) bool {
	return false
}

func (m *MdApi) Delete(key string) bool {
	return false
}

func (m *MdApi) Keys() []string {
	return []string{}
}
