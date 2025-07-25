/**
 * 
 * @doc
 * File system utilities.
 * TODO put this in api.utils
 */
declare module "fs"{
  /**
   * @param some The template string to render.
   * @param variable value map containing variables to be used in the template.
   * @returns A promise that resolves to the rendered string.  function bytes(path: string): bytes;
   */
  export function fs (some: any, variable: A key): any
  /**
   * 
   * Reads the contents of a file as a UTF-8 string.
   * 
   * @param path The file path to read.
   * @returns A promise that resolves to the file contents as a string.
   */
  export function fs (path: string): any
  /**
   * @param path The file path to read.
   * @param some The template string to render.
   * @param variable value map containing variables to be used in the template.
   * @returns A promise that resolves to the rendered string.  function bytes(template: string, variables: Record<string, any>): Promise<string>; iterable := vm.NewObject() iterable.SetSymbol(goja.SymIterator, vm.ToValue(func() goja.Value { index := 0 itr := vm.NewObject() itr.Set("next", vm.ToValue(func(call goja.FunctionCall) goja.Value { if index >= len(files) { result := IterEntry{Done: true} return vm.ToValue(result) }  file := files[index] index++ info, err := file.Info() if err != nil { panic(vm.NewTypeError("file.list", err)) }  time, err := vm.New(vm.Get("Date").ToObject(vm), vm.ToValue(info.ModTime())) if err != nil { panic(vm.NewTypeError("file.list", err)) } entry := FileEntry{ Name:        file.Name(), Path:        "/" + filepath.Join(dirPath, file.Name()), ModTime:     time, Size:        info.Size(), IsDirectory: file.IsDir(), IsFile:      file.Type().IsRegular(), IsSymlink:   file.Type()&os.ModeSymlink != 0, }  result := IterEntry{Done: false, Value: entry}  return vm.ToValue(result) }))  return itr }))  iterable.Set("toString", func(call goja.FunctionCall) goja.Value { return vm.ToValue(fmt.Sprintf("IterableObject(%v)", files)) }) return iterable No more entries Same as move since os.Rename works for directories
   */
  export function fs (path: any, some: any, variable: A key): any
}
/**
 * 
 * @doc
 * Markdown parser and renderer module.
 * Provides a `Parser` class to parse Markdown into HTML.
 */
declare module "md"{
  /**
   * 
   * Configuration object accepted by the `md.Parser` constructor.
   * 
   */
  export interface MdApiConfig { parser?: { NoIntraEmphasis?: boolean; Tables?: boolean; FencedCode?: boolean; Autolink?: boolean; Strikethrough?: boolean; LaxHTMLBlocks?: boolean; SpaceHeadings?: boolean; HardLineBreak?: boolean; NonBlockingSpace?: boolean; TabSizeEight?: boolean; Footnotes?: boolean; NoEmptyLineBeforeBlock?: boolean; HeadingIDs?: boolean; Titleblock?: boolean; AutoHeadingIDs?: boolean; BackslashLineBreak?: boolean; DefinitionLists?: boolean; MathJax?: boolean; OrderedListStart?: boolean; Attributes?: boolean; SuperSubscript?: boolean; EmptyLinesBreakList?: boolean; Includes?: boolean; Mmark?: boolean; }; renderer?: { HrefTargetBlank?: boolean; SkipHTML?: boolean; SkipImages?: boolean; SkipLinks?: boolean; Safelink?: boolean; NofollowLinks?: boolean; NoreferrerLinks?: boolean; NoopenerLinks?: boolean; CompletePage?: boolean; UseXHTML?: boolean; FootnoteReturnLinks?: boolean; FootnoteNoHRTag?: boolean; Smartypants?: boolean; SmartypantsFractions?: boolean; SmartypantsDashes?: boolean; SmartypantsLatexDashes?: boolean; SmartypantsAngledQuotes?: boolean; SmartypantsQuotesNBSP?: boolean; TOC?: boolean; LazyLoadImages?: boolean; }; }
  /**
   */
  export function constructor(options: MdApiConfig): md.Parser
  /**
   * 
   * Parses markdown to HTML.
   * 
   * @param markdown The Markdown content.
   * @returns The rendered HTML string.
   */
  export function render(markdown: string): string;
  /**
   * 
   * @doc
   * Class for parsing and rendering Markdown.
   * Configure with parser and renderer options.
   * 
   * @example
   * const parser = new md.Parser({ parser: { Tables: true }, renderer: { Smartypants: true } });
   * const html = parser.render("# Hello!");
   * 
   * Class for parsing and rendering Markdown.
   * Configure with parser and renderer options.
   * 
   */
  export class Parser{
  }
}
/**
 * 
 * @doc
 * A utility module for rendering and manipulating templates using the Plush system.
 * 
 * This module provides various functions that support rendering templates,
 * working with variables, and performing other templating tasks.
 * It uses the Plush template engine from Go (`github.com/gobuffalo/plush`).
 * 
 * Functions available in this module include `render`, which renders a template string
 * using the provided variables, and other utilities that may support different use cases.
 */
declare module "tpl"{
  /**
   * 
   * Renders a template using the provided variables, leveraging the Plush template system.
   * 
   * @param template A template string with embedded dynamic content (e.g., "<%= name %>").
   * @param variables value pairs to replace placeholders in the template.
   * @returns A promise that resolves to the rendered string with variables replaced.
   */
  export function render(template: string, variables: Record<string, any>): Promise<string>;
}
