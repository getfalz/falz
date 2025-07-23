package js

import (
	"github.com/dop251/goja"
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
	module.Set("Parser", NewMdApiConstructor(vm))
	return module
}

// @ts class md.Parser
//
// @default
//
// @doc
// Class for parsing and rendering Markdown.
// Configure with parser and renderer options.
//
// @example
// const parser = new md.Parser({ parser: { Tables: true }, renderer: { Smartypants: true } });
// const html = parser.render("# Hello!");
func NewMdApiConstructor(vm *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return nil
}

// @ts interface md.MdApiConfig
//
// @doc
// Configuration object accepted by the `md.Parser` constructor.
//
// @sig
//
//	interface MdApiConfig {
//	  parser?: {
//	    NoIntraEmphasis?: boolean;
//	    Tables?: boolean;
//	    FencedCode?: boolean;
//	    Autolink?: boolean;
//	    Strikethrough?: boolean;
//	    LaxHTMLBlocks?: boolean;
//	    SpaceHeadings?: boolean;
//	    HardLineBreak?: boolean;
//	    NonBlockingSpace?: boolean;
//	    TabSizeEight?: boolean;
//	    Footnotes?: boolean;
//	    NoEmptyLineBeforeBlock?: boolean;
//	    HeadingIDs?: boolean;
//	    Titleblock?: boolean;
//	    AutoHeadingIDs?: boolean;
//	    BackslashLineBreak?: boolean;
//	    DefinitionLists?: boolean;
//	    MathJax?: boolean;
//	    OrderedListStart?: boolean;
//	    Attributes?: boolean;
//	    SuperSubscript?: boolean;
//	    EmptyLinesBreakList?: boolean;
//	    Includes?: boolean;
//	    Mmark?: boolean;
//	  };
//	  renderer?: {
//	    HrefTargetBlank?: boolean;
//	    SkipHTML?: boolean;
//	    SkipImages?: boolean;
//	    SkipLinks?: boolean;
//	    Safelink?: boolean;
//	    NofollowLinks?: boolean;
//	    NoreferrerLinks?: boolean;
//	    NoopenerLinks?: boolean;
//	    CompletePage?: boolean;
//	    UseXHTML?: boolean;
//	    FootnoteReturnLinks?: boolean;
//	    FootnoteNoHRTag?: boolean;
//	    Smartypants?: boolean;
//	    SmartypantsFractions?: boolean;
//	    SmartypantsDashes?: boolean;
//	    SmartypantsLatexDashes?: boolean;
//	    SmartypantsAngledQuotes?: boolean;
//	    SmartypantsQuotesNBSP?: boolean;
//	    TOC?: boolean;
//	    LazyLoadImages?: boolean;
//	  };
//	}
type MdApiConfig struct {
	Parser   map[string]any
	Renderer map[string]any
}

// @ts method md.Parser.constructor
//
// @sig
// function constructor(options: MdApiConfig): md.Parser
func NewMdApi(vm *goja.Runtime, options MdApiConfig) *MdApi {
	return nil
}

// @ts method md.Parser.render
//
// @doc
// Parses markdown to HTML.
//
// @param markdown - The Markdown content.
// @returns The rendered HTML string.
//
// @sig
// function render(markdown: string): string;
func (m *MdApi) Parse(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
	return nil
}
