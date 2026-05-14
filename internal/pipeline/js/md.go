package js

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// @ts namespace md
//
// @doc
func GetMdModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("toHtml", func(call goja.FunctionCall) goja.Value { return mdToHtml(vm, call) })

	commonOpts := MdApiConfig{
		Parser:   commonParserOpts,
		Renderer: commonRendererOpts,
	}
	module.Set("commonOptions", commonOpts)
	return module
}

// @ts function md.toHtml
//
// @doc
// Parses markdown to HTML.
//
// @param markdown - string - The Markdown content.
// @param options? - Config - Configuration object for the parser and renderer.
// @returns string - The rendered HTML string.
func mdToHtml(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	src, err := argMustString(call.Argument(0))
	if err != nil {
		panic(vm.NewTypeError("md.toHtml: first argument must be a string: %v", err))
	}

	var parserExt parser.Extensions
	var renderFlags html.Flags

	if arg1 := call.Argument(1); !goja.IsUndefined(arg1) && !goja.IsNull(arg1) {
		obj, ok := arg1.(*goja.Object)
		if !ok {
			panic(vm.NewTypeError("md.toHtml: options must be an object, got %T", arg1))
		}
		parserExt, err = extractParserExtensions(vm, obj)
		if err != nil {
			panic(vm.NewTypeError("md.toHtml: invalid parser options: %v", err))
		}
		renderFlags, err = extractRenderFlags(vm, obj)
		if err != nil {
			panic(vm.NewTypeError("md.toHtml: invalid renderer options: %v", err))
		}
	}

	pa := parser.NewWithExtensions(parserExt)
	ren := html.NewRenderer(html.RendererOptions{Flags: renderFlags})
	out := markdown.Render(pa.Parse([]byte(src)), ren)

	return vm.ToValue(string(out))
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
func extractParserExtensions(vm *goja.Runtime, arg *goja.Object) (parser.Extensions, error) {
	raw, ex := GetSafe(vm, arg, "parser")
	if ex != nil {
		return parser.NoExtensions, fmt.Errorf("reading parser key: %v", ex)
	}
	if raw == nil || goja.IsUndefined(raw) || goja.IsNull(raw) {
		return parser.NoExtensions, nil
	}
	options, ok := raw.ToObject(vm).Export().(map[string]any)
	if !ok {
		return parser.NoExtensions, fmt.Errorf("parser options must be an object")
	}

	ext := parser.NoExtensions
	for key, val := range options {
		if on, ok := val.(bool); !ok || !on {
			continue
		}
		switch key {
		case "NoIntraEmphasis":
			ext |= parser.NoIntraEmphasis
		case "Tables":
			ext |= parser.Tables
		case "FencedCode":
			ext |= parser.FencedCode
		case "Autolink":
			ext |= parser.Autolink
		case "Strikethrough":
			ext |= parser.Strikethrough
		case "LaxHTMLBlocks":
			ext |= parser.LaxHTMLBlocks
		case "SpaceHeadings":
			ext |= parser.SpaceHeadings
		case "HardLineBreak":
			ext |= parser.HardLineBreak
		case "NonBlockingSpace":
			ext |= parser.NonBlockingSpace
		case "TabSizeEight":
			ext |= parser.TabSizeEight
		case "Footnotes":
			ext |= parser.Footnotes
		case "NoEmptyLineBeforeBlock":
			ext |= parser.NoEmptyLineBeforeBlock
		case "HeadingIDs":
			fmt.Println("getParserExtensions: enabling HeadingIDs")
			ext |= parser.HeadingIDs
		case "Titleblock":
			ext |= parser.Titleblock
		case "AutoHeadingIDs":
			ext |= parser.AutoHeadingIDs
		case "BackslashLineBreak":
			ext |= parser.BackslashLineBreak
		case "DefinitionLists":
			ext |= parser.DefinitionLists
		case "MathJax":
			ext |= parser.MathJax
		case "OrderedListStart":
			ext |= parser.OrderedListStart
		case "Attributes":
			ext |= parser.Attributes
		case "SuperSubscript":
			ext |= parser.SuperSubscript
		case "EmptyLinesBreakList":
			ext |= parser.EmptyLinesBreakList
		case "Includes":
			ext |= parser.Includes
		case "Mmark":
			ext |= parser.Mmark
		}
	}
	return ext, nil
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
func extractRenderFlags(vm *goja.Runtime, arg *goja.Object) (html.Flags, error) {
	raw, ex := GetSafe(vm, arg, "renderer")
	if ex != nil {
		return html.FlagsNone, fmt.Errorf("reading renderer key: %v", ex)
	}
	if raw == nil || goja.IsUndefined(raw) || goja.IsNull(raw) {
		return html.FlagsNone, nil
	}
	options, ok := raw.ToObject(vm).Export().(map[string]any)
	if !ok {
		return html.FlagsNone, fmt.Errorf("renderer options must be an object")
	}

	renderFlags := html.FlagsNone
	for key, val := range options {
		if on, ok := val.(bool); !ok || !on {
			continue
		}
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
	return renderFlags, nil
}

// @ts interface md.Config
//
// @doc
// Top-level options object for md.toHtml.
//
// @field parser? - ParserOptions - Controls which markdown extensions the parser recognises.
// @field renderer? - RendererOptions - Controls HTML output flags.
type MdApiConfig struct {
	Parser   map[string]bool
	Renderer map[string]bool
}

// @ts const md.commonOptions
//
// @type Config
//
// @doc
// A ready-to-use Config value enabling the most commonly needed parser extensions
// and renderer flags
//
// Enabled parser extensions: NoIntraEmphasis, Tables, FencedCode, Autolink,
// Strikethrough, SpaceHeadings, HeadingIDs, BackslashLineBreak, DefinitionLists, MathJax.
//
// Enabled renderer flags: Smartypants, SmartypantsFractions,
// SmartypantsDashes, SmartypantsLatexDashes.
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
