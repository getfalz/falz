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
}

// @ts namespace md
//
// @doc
// Markdown parser and renderer module.
// Provides a `Parser` class to parse Markdown into HTML.
func GetMdModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	module.Set("Parser", NewParser(vm))

	commonOpts := MdApiConfig{
		Parser:   commonParserOpts,
		Renderer: commonRendererOpts,
	}
	module.Set("commonOptions", commonOpts)
	return module
}

// @ts class md.Parser
//
// @doc
// Class for parsing and rendering Markdown.
// Configure with parser and renderer options.
//
// @example
// const parser = new md.Parser({ parser: { Tables: true }, renderer: { Smartypants: true } });
// const html = parser.render("# Hello!");
func NewParser(vm *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("Parser requires a configuration object"))
		}

		optionsArg := call.Argument(0).ToObject(vm)
		config := MdApiConfig{
			Parser:   make(map[string]bool),
			Renderer: make(map[string]bool),
		}

		if parserConfig := optionsArg.Get("parser"); parserConfig != nil {
			if parserObj, ok := parserConfig.ToObject(vm).Export().(map[string]bool); ok {
				config.Parser = parserObj
			}
		}

		if rendererConfig := optionsArg.Get("renderer"); rendererConfig != nil {
			if rendererObj, ok := rendererConfig.ToObject(vm).Export().(map[string]bool); ok {
				config.Renderer = rendererObj
			}
		}

		mdApi := NewMdApi(vm, config)

		return vm.ToValue(mdApi).ToObject(vm)
	}
}

// @ts interface md.ParserOptions
//
// @doc
// This config sets which extensions are enabled when parsing Markdown.
//
// @sig
//
//	interface ParserOptions {
//	  /** Ignore emphasis markers inside words */
//	  NoIntraEmphasis?: boolean;
//	  /** Parse tables */
//	  Tables?: boolean;
//	  /** Parse fenced code blocks */
//	  FencedCode?: boolean;
//	  /** Detect embedded URLs that are not explicitly marked */
//	  Autolink?: boolean;
//	  /** Strikethrough text using ~~test~~ */
//	  Strikethrough?: boolean;
//	  /** Loosen up HTML block parsing rules */
//	  LaxHTMLBlocks?: boolean;
//	  /** Be strict about prefix heading rules */
//	  SpaceHeadings?: boolean;
//	  /** Translate newlines into line breaks */
//	  HardLineBreak?: boolean;
//	  /** Translate backspace spaces into non-blocking spaces */
//	  NonBlockingSpace?: boolean;
//	  /** Expand tabs to eight spaces instead of four */
//	  TabSizeEight?: boolean;
//	  /** Pandoc-style footnotes */
//	  Footnotes?: boolean;
//	  /** No need to insert an empty line to start a block (code, quote, lists) */
//	  NoEmptyLineBeforeBlock?: boolean;
//	  /** Specify heading IDs with {#id} */
//	  HeadingIDs?: boolean;
//	  /** Titleblock ala pandoc */
//	  Titleblock?: boolean;
//	  /** Create the heading ID from the text */
//	  AutoHeadingIDs?: boolean;
//	  /** Translate trailing backslashes into line breaks */
//	  BackslashLineBreak?: boolean;
//	  /** Parse definition lists */
//	  DefinitionLists?: boolean;
//	  /** Parse MathJax */
//	  MathJax?: boolean;
//	  /** Keep track of the first number when starting an ordered list */
//	  OrderedListStart?: boolean;
//	  /** Block attributes */
//	  Attributes?: boolean;
//	  /** Super- and subscript support: 2^10^, H~2~O */
//	  SuperSubscript?: boolean;
//	  /** Two empty lines break out of a list */
//	  EmptyLinesBreakList?: boolean;
//	  /** Support including other files */
//	  Includes?: boolean;
//	  /** Support Mmark syntax (see https://mmark.miek.nl/post/syntax/) */
//	  Mmark?: boolean;
//	}

// @ts interface md.RendererOptions
//
// @doc
// This config sets which extensions are enabled when rendering Markdown.
//
// @sig
//
//	interface RendererOptions {
//	  /** Skip preformatted HTML blocks */
//	  SkipHTML?: boolean;
//	  /** Skip embedded images */
//	  SkipImages?: boolean;
//	  /** Skip all links */
//	  SkipLinks?: boolean;
//	  /** Only link to trusted protocols */
//	  Safelink?: boolean;
//	  /** Only link with rel="nofollow" */
//	  NofollowLinks?: boolean;
//	  /** Only link with rel="noreferrer" */
//	  NoreferrerLinks?: boolean;
//	  /** Only link with rel="noopener" */
//	  NoopenerLinks?: boolean;
//	  /** Add a blank target */
//	  HrefTargetBlank?: boolean;
//	  /** Generate a complete HTML page */
//	  CompletePage?: boolean;
//	  /** Generate XHTML output instead of HTML */
//	  UseXHTML?: boolean;
//	  /** Generate a link at the end of a footnote to return to the source */
//	  FootnoteReturnLinks?: boolean;
//	  /** Do not output an HR after starting a footnote list */
//	  FootnoteNoHRTag?: boolean;
//	  /** Enable smart punctuation substitutions */
//	  Smartypants?: boolean;
//	  /** Enable smart fractions (with Smartypants) */
//	  SmartypantsFractions?: boolean;
//	  /** Enable smart dashes (with Smartypants) */
//	  SmartypantsDashes?: boolean;
//	  /** Enable LaTeX-style dashes (with Smartypants) */
//	  SmartypantsLatexDashes?: boolean;
//	  /** Enable angled double quotes for double quotes rendering */
//	  SmartypantsAngledQuotes?: boolean;
//	  /** Enable « French guillemets » (with Smartypants) */
//	  SmartypantsQuotesNBSP?: boolean;
//	  /** Generate a table of contents */
//	  TOC?: boolean;
//	  /** Include loading="lazy" with images */
//	  LazyLoadImages?: boolean;
//	}

// @ts interface md.Config
//
// @doc
// This config sets which extensions are enabled when rendering Markdown.
//
// @sig
//
//	interface Config {
//	  /** Parser options */
//	  parser?: ParserOptions;
//	  /** Renderer options */
//	  renderer?: RendererOptions;
//	}
type MdApiConfig struct {
	Parser   map[string]bool
	Renderer map[string]bool
}

// @ts interface md.CommonOptions
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
//
// @sig
// export const commonOptions: Config;
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

// @ts constructor md.Parser.constructor
//
// @param options - Config - Configuration object for the parser and renderer.
// @returns md.Parser - A new instance of `md.Parser`.
func NewMdApi(vm *goja.Runtime, options MdApiConfig) *MdApi {
	parserExtensions := parser.CommonExtensions
	rendererFlags := html.CommonFlags

	for key, enabled := range options.Parser {
		if enabled {
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

	for key, enabled := range options.Renderer {
		if enabled {
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

// @ts method md.Parser.render
//
// @doc
// Parses markdown to HTML.
//
// @param markdown - string - The Markdown content.
// @returns Promise<string> - The rendered HTML string.
func (m *MdApi) Render(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		if len(call.Arguments) < 1 {
			reject(fmt.Errorf("render requires a one argument"))
			return
		}

		arg := call.Arguments[0]

		data, err := GetByteArg(vm, arg)
		if err != nil {
			reject(fmt.Errorf("render argument error: %w", err))
			return
		}

		doc := m.parser.Parse(data)
		html := markdown.Render(doc, m.renderer)

		resolve(vm.ToValue(string(html)))
	}()

	return vm.ToValue(promise)
}
