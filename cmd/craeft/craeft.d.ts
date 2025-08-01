declare module "fasttpl" {
  /**
   * 
   * This config sets which extensions are enabled when rendering Markdown.
   * 
   *
   */
  interface Config {
    /**
     * Opening tag for variable substitution in templates. Defaults to "{{"
     */
    startTag: string
    /**
     * Closing tag for variable substitution in templates. Defaults to "}}"
     */
    endTag: string
  }
  /**
   * 
   * Renders a template using the provided variables.
   * 
   * @param template A template string with embedded dynamic content (e.g., "<%= name %>").
   * @param variables An object (map) containing key-value pairs to replace placeholders in the template.
   * @param config? An object (map) containing key-value pairs to replace placeholders in the template.
   * @returns A promise that resolves to the rendered string with variables replaced.
   *
   */
  export function compile (template: string, variables: Record<string, any>, config?: Config): Promise<string>
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
   * This function opens a file.
   * 
   * @param path The template string to render.
   * @returns A promise that resolves to the rendered string.
   *
   */
  export function bytes (path: string): Promise<string>
  /**
   * 
   * Reads the contents of a file as a UTF-8 string.
   * 
   * @param path The template string to render.
   * @returns A promise that resolves to the rendered string.
   *
   * @example
   * 
   * const text = await fs.text("files/hello.txt");
   * console.log(text);
   */
  export function text (path: string): Promise<string>
  /**
   * 
   * Reads and parses a JSON file.
   * 
   * @param path The file path to read.
   * @returns A promise that resolves to the parsed JSON object.
   *
   * @example
   * 
   * const config = await fs.json("/path/to/config.json");
   * console.log(config.apiKey);
   */
  export function json (path: string): Promise<Object>
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
   * This config sets which extensions are enabled when rendering Markdown.
   * 
   *
   */
  interface Config {
    /**
     * Generate a table of contents
     */
    parser?: ParserOptions
    /**
     * Renderer options
     */
    renderer?: RendererOptions
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
  export function toHtml (markdown: string, options?: Config): Promise<string>
  /**
   * 
   * Common options for markdown parsing and rendering.
   * 
   * **Parser flags**
   * - `NoIntraEmphasis`
   * - `Tables`
   * - `FencedCode`
   * - `Autolink`
   * - `Strikethrough`
   * - `SpaceHeadings`
   * - `HeadingIDs`
   * - `BackslashLineBreak`
   * - `DefinitionLists`
   * - `MathJax`
   * 
   * **Renderer flags**
   * - `Smartypants`
   * - `SmartypantsFractions`
   * - `SmartypantsDashes`
   * - `SmartypantsLatexDashes`
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
   * Renders a template using the provided variables, leveraging the Plush template system.
   * 
   * @param template A template string with embedded dynamic content (e.g., "<%= name %>").
   * @param variables An object (map) containing key-value pairs to replace placeholders in the template.
   * @returns A promise that resolves to the rendered string with variables replaced.
   *
   */
  export function compile (template: string, variables: Record<string, any>): Promise<string>
}
