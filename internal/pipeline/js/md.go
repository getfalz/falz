package js

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
	ev       *EventLoop
}

// @ts namespace md
//
// @doc
// Markdown parser and renderer module.
// Provides a `Parser` class to parse Markdown into HTML.
func GetMdModule(ev *EventLoop) *goja.Object {
	module := ev.vm.NewObject()
	module.Set("render", func(call goja.FunctionCall) goja.Value { return MdRender(call, ev) })

	commonOpts := MdApiConfig{
		Parser:   commonParserOpts,
		Renderer: commonRendererOpts,
	}
	module.Set("commonOptions", commonOpts)
	return module
}

// @ts function md.render
//
// @doc
// Parses markdown to HTML.
//
// @param markdown - string - The Markdown content.
// @param options? - Config - Configuration object for the parser and renderer.
// @returns Promise<string> - The rendered HTML string.
func MdRender(call goja.FunctionCall, ev *EventLoop) goja.Value {
	promise, resolve, reject := ev.vm.NewPromise()

	arg0 := call.Argument(0)
	arg1 := call.Argument(1)

	go func() {
		mdcontent, err := GetByteArg(ev.vm, arg0)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError(
					"First argument to md.render must be a string: %v", err,
				))
			})
			return
		}

		objRaw, ok := arg1.(*goja.Object)
		if !ok {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError(
					"Second argument to md.render must be an object, got '%v'", arg1,
				))
			})
			return
		}

		htmlFlags, err := getRenderFlags(objRaw, ev.vm)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError(
					"could not get renderer flags: %v", err,
				))
			})
			return
		}
		parserExt, err := getParserExtensions(objRaw, ev.vm)
		if err != nil {
			ev.RunOnLoop(func(vm *goja.Runtime) {
				reject(ev.vm.NewTypeError(
					"could not get parser extensions: %v", err,
				))
			})
			return
		}

		pa := parser.NewWithExtensions(parserExt)
		opts := html.RendererOptions{Flags: htmlFlags}
		renderer := html.NewRenderer(opts)

		doc := pa.Parse(mdcontent)
		html := markdown.Render(doc, renderer)

		ev.RunOnLoop(func(vm *goja.Runtime) {
			resolve(string(html))
		})
	}()

	return ev.vm.ToValue(promise)
}

// @ts interface md.ParserOptions
//
// @doc
// This config sets which extensions are enabled when parsing Markdown.
//
// @field NoIntraEmphasis?: boolean - Ignore emphasis markers inside words
// @field Tables?: boolean - Parse tables
// @field FencedCode?: boolean - Parse fenced code blocks
// @field Autolink?: boolean - Detect embedded URLs that are not explicitly marked
// @field Strikethrough?: boolean - Strikethrough text using ~~test~~
// @field LaxHTMLBlocks?: boolean - Loosen up HTML block parsing rules
// @field SpaceHeadings?: boolean - Be strict about prefix heading rules
// @field HardLineBreak?: boolean - Translate newlines into line breaks
// @field NonBlockingSpace?: boolean - Translate backspace spaces into non-blocking spaces
// @field TabSizeEight?: boolean - Expand tabs to eight spaces instead of four
// @field Footnotes?: boolean - Pandoc-style footnotes
// @field NoEmptyLineBeforeBlock?: boolean - No need to insert an empty line to start a block (code, quote, lists)
// @field HeadingIDs?: boolean - Specify heading IDs with {#id}
// @field Titleblock?: boolean - Titleblock ala pandoc
// @field AutoHeadingIDs?: boolean - Create the heading ID from the text
// @field BackslashLineBreak?: boolean - Translate trailing backslashes into line breaks
// @field DefinitionLists?: boolean - Parse definition lists
// @field MathJax?: boolean - Parse MathJax
// @field OrderedListStart?: boolean - Keep track of the first number when starting an ordered list
// @field Attributes?: boolean - Block attributes
// @field SuperSubscript?: boolean - Super- and subscript support: 2^10^, H~2~O
// @field EmptyLinesBreakList?: boolean - Two empty lines break out of a list
// @field Includes?: boolean - Support including other files
// @field Mmark?: boolean - Support Mmark syntax (see https://mmark.miek.nl/post/syntax/)
func getParserExtensions(arg *goja.Object, vm *goja.Runtime) (parser.Extensions, error) {
	options := make(map[string]any)

	parserConfig, exception := GetSafe(vm, arg, "parser")

	if exception != nil {
		return parser.NoExtensions, fmt.Errorf("could not get parser options: %v", exception)
	}

	if parserConfig != nil {
		if parserObj, ok := parserConfig.ToObject(vm).Export().(map[string]any); ok {
			options = parserObj
		}
	} else {
		return parser.NoExtensions, fmt.Errorf("could not convert parser options to object, got: '%v'", parserConfig)
	}

	parserExtensions := parser.NoExtensions
	for key, enabled := range options {
		if enabled, ok := enabled.(bool); ok && enabled {
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
				fmt.Println("getParserExtensions: enabling HeadingIDs")
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
	return parserExtensions, nil
}

// @ts interface md.RendererOptions
//
// @doc
// This config sets which extensions are enabled when rendering Markdown.
//
// @field SkipHTML?: boolean - Skip preformatted HTML blocks
// @field SkipImages?: boolean - Skip embedded images
// @field SkipLinks?: boolean - Skip all links
// @field Safelink?: boolean - Only link to trusted protocols
// @field NofollowLinks?: boolean - Only link with rel="nofollow"
// @field NoreferrerLinks?: boolean - Only link with rel="noreferrer"
// @field NoopenerLinks?: boolean - Only link with rel="noopener"
// @field HrefTargetBlank?: boolean - Add a blank target
// @field CompletePage?: boolean - Generate a complete HTML page
// @field UseXHTML?: boolean - Generate XHTML output instead of HTML
// @field FootnoteReturnLinks?: boolean - Generate a link at the end of a footnote to return to the source
// @field FootnoteNoHRTag?: boolean - Do not output an HR after starting a footnote list
// @field Smartypants?: boolean - Enable smart punctuation substitutions
// @field SmartypantsFractions?: boolean - Enable smart fractions (with Smartypants)
// @field SmartypantsDashes?: boolean - Enable smart dashes (with Smartypants)
// @field SmartypantsLatexDashes?: boolean - Enable LaTeX-style dashes (with Smartypants)
// @field SmartypantsAngledQuotes?: boolean - Enable angled double quotes for double quotes rendering
// @field SmartypantsQuotesNBSP?: boolean - Enable « French guillemets » (with Smartypants)
// @field TOC?: boolean - Generate a table of contents
// @field LazyLoadImages?: boolean - Include loading="lazy" with images
func getRenderFlags(arg *goja.Object, vm *goja.Runtime) (html.Flags, error) {
	options := make(map[string]any)

	parserConfig, exception := GetSafe(vm, arg, "renderer")
	if exception != nil {
		return html.FlagsNone, fmt.Errorf("could not get renderer options: %v", exception)
	}

	if parserConfig != nil {
		if parserObj, ok := parserConfig.ToObject(vm).Export().(map[string]any); ok {
			options = parserObj
		}
	} else {
		return html.FlagsNone, fmt.Errorf("renderer options must be an object, got: '%v'", parserConfig)
	}

	renderFlags := html.FlagsNone
	for key, enabled := range options {
		if enabled, ok := enabled.(bool); ok && enabled {
			switch key {
			case "HrefTargetBlank":
				renderFlags |= html.HrefTargetBlank
			case "SkipHTML":
				renderFlags |= html.SkipHTML
			case "SkipImages":
				renderFlags |= html.SkipImages
			case "SkipLinks":
				renderFlags |= html.SkipLinks
			case "Safelink":
				renderFlags |= html.Safelink
			case "NofollowLinks":
				renderFlags |= html.NofollowLinks
			case "NoreferrerLinks":
				renderFlags |= html.NoreferrerLinks
			case "NoopenerLinks":
				renderFlags |= html.NoopenerLinks
			case "CompletePage":
				renderFlags |= html.CompletePage
			case "UseXHTML":
				renderFlags |= html.UseXHTML
			case "FootnoteReturnLinks":
				renderFlags |= html.FootnoteReturnLinks
			case "FootnoteNoHRTag":
				renderFlags |= html.FootnoteNoHRTag
			case "Smartypants":
				renderFlags |= html.Smartypants
			case "SmartypantsFractions":
				renderFlags |= html.SmartypantsFractions
			case "SmartypantsDashes":
				renderFlags |= html.SmartypantsDashes
			case "SmartypantsLatexDashes":
				renderFlags |= html.SmartypantsLatexDashes
			case "SmartypantsAngledQuotes":
				renderFlags |= html.SmartypantsAngledQuotes
			case "SmartypantsQuotesNBSP":
				renderFlags |= html.SmartypantsQuotesNBSP
			case "TOC":
				renderFlags |= html.TOC
			case "LazyLoadImages":
				renderFlags |= html.LazyLoadImages
			}
		}
	}
	return renderFlags, nil
}

// @ts interface md.Config
//
// @doc
// This config sets which extensions are enabled when rendering Markdown.
//
// @field parser?: ParserOptions - Generate a table of contents
// @field renderer?: RendererOptions - Renderer options
type MdApiConfig struct {
	Parser   map[string]bool
	Renderer map[string]bool
}

// @ts const md.commonOptions
//
// @type Config
//
// @doc
// Common options for markdown parsing and rendering.
//
// **Parser flags**
// - `NoIntraEmphasis`
// - `Tables`
// - `FencedCode`
// - `Autolink`
// - `Strikethrough`
// - `SpaceHeadings`
// - `HeadingIDs`
// - `BackslashLineBreak`
// - `DefinitionLists`
// - `MathJax`
//
// **Renderer flags**
// - `Smartypants`
// - `SmartypantsFractions`
// - `SmartypantsDashes`
// - `SmartypantsLatexDashes`
var commonParserOpts = map[string]bool{
	"NoIntraEmphasis":    true,
	"Tables":             true,
	"FencedCode":         true,
	"Autolink":           true,
	"Strikethrough":      true,
	"SpaceHeadings":      true,
	"HeadingIDs":         true,
	"BackslashLineBreak": true,
	"DefinitionLists":    true,
	"MathJax":            true,
}

var commonRendererOpts = map[string]bool{
	"Smartypants":            true,
	"SmartypantsFractions":   true,
	"SmartypantsDashes":      true,
	"SmartypantsLatexDashes": true,
}
