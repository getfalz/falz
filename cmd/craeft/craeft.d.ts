/**
 * 
 * @doc
 * Synchronous template rendering with support for string and function-valued parameters.
 *
 */
declare module "fasttpl" {
  interface Config {
    /**
     * string - Opening delimiter. Default: "{{"
     */
    startTag?
    /**
     * string - Closing delimiter. Default: "}}"
     */
    endTag?
  }
  /**
   * 
   * Renders a template by substituting tagged placeholders with values from the
   * parameters object. Parameter values may be plain strings or zero-argument
   * functions that return strings (evaluated lazily per occurrence).
   * 
   * @param template Template string, e.g. "Hello {{name}}!"
   * @param params Substitution values.
   * @param config? Optional tag delimiters.
   * @returns The rendered string.
   *
   */
  export function compile (template: string, params: Record<string, string | (() => string)>, config?: fasttpl.Config): string
}
/**
 * 
 * @doc
 * File system utilities.
 *
 */
declare module "fs" {
  /**
   * 
   * Represents a single filesystem entry as returned by fs.list and fs.walk.
   * The path field is always slash-separated and prefixed with /, relative to the fs root.
   * 
   *
   */
  interface FileEntry {
    /**
     * string - The entry's filename without any directory component.
     */
    name
    /**
     * string - Full path relative to the fs root, slash-separated, with leading slash.
     */
    path
    /**
     * number - Size in bytes. Always 0 for directories.
     */
    size
    /**
     * Date - Last modification time as a JavaScript Date.
     */
    modTime
    /**
     * boolean - True if the entry is a regular file.
     */
    isFile
    /**
     * boolean - True if the entry is a directory.
     */
    isDirectory
    /**
     * boolean - True if the entry is a symbolic link.
     */
    isSymlink
  }
  /**
   * 
   * Reads a file and returns its contents as a Uint8Array.
   * Returns undefined if the file does not exist.
   * Throws an Error for all other I/O failures.
   * 
   * @param path Path relative to the fs root.
   * @returns Uint8Array | undefined
   *
   */
  export function bytes (path: string): any
  /**
   * 
   * Reads a file and returns its contents as a UTF-8 string.
   * Returns undefined if the file does not exist.
   * Throws an Error for all other I/O failures.
   * 
   * @param path Path relative to the fs root.
   * @returns string | undefined
   *
   */
  export function text (path: string): any
  /**
   * 
   * Reads and parses a JSON file, returning the parsed value.
   * Returns undefined if the file does not exist.
   * Throws a SyntaxError if the file content is not valid JSON.
   * Throws an Error for all other I/O failures.
   * 
   * @param path Path relative to the fs root.
   * @returns any | undefined
   *
   */
  export function json (path: string): any
  /**
   * 
   * Reports whether a path exists (as a file, directory, or any other entry).
   * 
   * @param path Path relative to the fs root.
   * @returns boolean
   *
   */
  export function exists (path: string): any
  /**
   * 
   * Reports whether the path refers to a regular file.
   * Returns false if the path does not exist or is not a file.
   * 
   * @param path Path relative to the fs root.
   * @returns boolean
   *
   */
  export function isFile (path: string): any
  /**
   * 
   * Reports whether the path refers to a directory.
   * Returns false if the path does not exist or is not a directory.
   * 
   * @param path Path relative to the fs root.
   * @returns boolean
   *
   */
  export function isDirectory (path: string): any
  /**
   * 
   * Returns the size of a file in bytes.
   * Returns undefined if the file does not exist.
   * Throws an Error for all other I/O failures.
   * 
   * @param path Path relative to the fs root.
   * @returns number | undefined
   *
   */
  export function size (path: string): any
  /**
   * 
   * Returns the last modification time of a file as a JavaScript Date object.
   * Returns undefined if the file does not exist.
   * Throws an Error for all other I/O failures.
   * 
   * @param path Path relative to the fs root.
   * @returns Date | undefined
   *
   */
  export function lastModified (path: string): any
  /**
   * 
   * Returns the direct children of a directory as an array of FileEntry objects.
   * The directory itself is not included. Entries are in filesystem order (typically alphabetical).
   * Does not recurse into subdirectories — use fs.walk for recursive traversal.
   * Throws an Error if the path does not exist or is not a directory.
   * 
   * @param path Path relative to the fs root.
   * @returns FileEntry[]
   *
   */
  export function list (path: string): any
  /**
   * 
   * Recursively traverses a directory and returns all entries (files and subdirectories)
   * as a flat array of FileEntry objects. The root path itself is not included.
   * Entries are visited in lexical order. Symlinks are not followed.
   * Throws an Error if the root path does not exist or is not a directory.
   * 
   * @param path Path relative to the fs root.
   * @returns FileEntry[]
   *
   */
  export function walk (path: string): any
}
/**
 * 
 * @doc
 *
 */
declare module "md" {
  /**
   * 
   * This config sets which extensions are enabled when parsing Markdown.
   * 
   *
   */
  interface ParserOptions {
    /**
     * Ignore emphasis markers inside words
     */
    NoIntraEmphasis?: boolean
    /**
     * Parse tables
     */
    Tables?: boolean
    /**
     * Parse fenced code blocks
     */
    FencedCode?: boolean
    /**
     * Detect embedded URLs that are not explicitly marked
     */
    Autolink?: boolean
    /**
     * Strikethrough text using ~~test~~
     */
    Strikethrough?: boolean
    /**
     * Loosen up HTML block parsing rules
     */
    LaxHTMLBlocks?: boolean
    /**
     * Be strict about prefix heading rules
     */
    SpaceHeadings?: boolean
    /**
     * Translate newlines into line breaks
     */
    HardLineBreak?: boolean
    /**
     * Translate backspace spaces into non-blocking spaces
     */
    NonBlockingSpace?: boolean
    /**
     * Expand tabs to eight spaces instead of four
     */
    TabSizeEight?: boolean
    /**
     * Pandoc-style footnotes
     */
    Footnotes?: boolean
    /**
     * No need to insert an empty line to start a block (code, quote, lists)
     */
    NoEmptyLineBeforeBlock?: boolean
    /**
     * Specify heading IDs with {#id}
     */
    HeadingIDs?: boolean
    /**
     * Titleblock ala pandoc
     */
    Titleblock?: boolean
    /**
     * Create the heading ID from the text
     */
    AutoHeadingIDs?: boolean
    /**
     * Translate trailing backslashes into line breaks
     */
    BackslashLineBreak?: boolean
    /**
     * Parse definition lists
     */
    DefinitionLists?: boolean
    /**
     * Parse MathJax
     */
    MathJax?: boolean
    /**
     * Keep track of the first number when starting an ordered list
     */
    OrderedListStart?: boolean
    /**
     * Block attributes
     */
    Attributes?: boolean
    /**
     * Super- and subscript support: 2^10^, H~2~O
     */
    SuperSubscript?: boolean
    /**
     * Two empty lines break out of a list
     */
    EmptyLinesBreakList?: boolean
    /**
     * Support including other files
     */
    Includes?: boolean
    /**
     * Support Mmark syntax (see https://mmark.miek.nl/post/syntax/)
     */
    Mmark?: boolean
  }
  /**
   * 
   * This config sets which extensions are enabled when rendering Markdown.
   * 
   *
   */
  interface RendererOptions {
    /**
     * Skip preformatted HTML blocks
     */
    SkipHTML?: boolean
    /**
     * Skip embedded images
     */
    SkipImages?: boolean
    /**
     * Skip all links
     */
    SkipLinks?: boolean
    /**
     * Only link to trusted protocols
     */
    Safelink?: boolean
    /**
     * Only link with rel="nofollow"
     */
    NofollowLinks?: boolean
    /**
     * Only link with rel="noreferrer"
     */
    NoreferrerLinks?: boolean
    /**
     * Only link with rel="noopener"
     */
    NoopenerLinks?: boolean
    /**
     * Add a blank target
     */
    HrefTargetBlank?: boolean
    /**
     * Generate a complete HTML page
     */
    CompletePage?: boolean
    /**
     * Generate XHTML output instead of HTML
     */
    UseXHTML?: boolean
    /**
     * Generate a link at the end of a footnote to return to the source
     */
    FootnoteReturnLinks?: boolean
    /**
     * Do not output an HR after starting a footnote list
     */
    FootnoteNoHRTag?: boolean
    /**
     * Enable smart punctuation substitutions
     */
    Smartypants?: boolean
    /**
     * Enable smart fractions (with Smartypants)
     */
    SmartypantsFractions?: boolean
    /**
     * Enable smart dashes (with Smartypants)
     */
    SmartypantsDashes?: boolean
    /**
     * Enable LaTeX-style dashes (with Smartypants)
     */
    SmartypantsLatexDashes?: boolean
    /**
     * Enable angled double quotes for double quotes rendering
     */
    SmartypantsAngledQuotes?: boolean
    /**
     * Enable « French guillemets » (with Smartypants)
     */
    SmartypantsQuotesNBSP?: boolean
    /**
     * Generate a table of contents
     */
    TOC?: boolean
    /**
     * Include loading="lazy" with images
     */
    LazyLoadImages?: boolean
  }
  /**
   * 
   * Top-level options object for md.toHtml.
   * 
   *
   */
  interface Config {
    /**
     * ParserOptions - Controls which markdown extensions the parser recognises.
     */
    parser?
    /**
     * RendererOptions - Controls HTML output flags.
     */
    renderer?
  }
  /**
   * 
   * Parses markdown to HTML.
   * 
   * @param markdown The Markdown content.
   * @param options? Configuration object for the parser and renderer.
   * @returns The rendered HTML string.
   *
   */
  export function toHtml (markdown: string, options?: Config): string
  /**
   * 
   * A ready-to-use Config value enabling the most commonly needed parser extensions
   * and renderer flags
   * 
   * Enabled parser extensions: NoIntraEmphasis, Tables, FencedCode, Autolink,
   * Strikethrough, SpaceHeadings, HeadingIDs, BackslashLineBreak, DefinitionLists, MathJax.
   * 
   * Enabled renderer flags: Smartypants, SmartypantsFractions,
   * SmartypantsDashes, SmartypantsLatexDashes.
   *
   */
  export const commonOptions: Config
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
 *
 */
declare module "tpl" {
  /**
   * 
   * Renders a Plush template string against a variables map and returns the result.
   * Plush supports ERB-style tags:
   * - <%= expr %>  outputs the result of expr (HTML-escaped by default)
   * - <%== expr %> outputs the result of expr without escaping
   * - <% stmt %>   executes a statement (if, for, etc.) without output
   * 
   * Throws a TypeError if arguments have the wrong type.
   * Throws an Error if template rendering fails (syntax error, undefined variable, etc.).
   * 
   * @param template A template string with embedded dynamic content (e.g., "<%= name %>").
   * @param variables An object (map) containing key-value pairs to replace placeholders in the template.
   * @returns A promise that resolves to the rendered string with variables replaced.
   *
   */
  export function render (template: string, variables: Record<string, any>): string
}
