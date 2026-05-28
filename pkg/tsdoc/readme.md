# `tsdoc` — TypeScript declaration generator from Go comments

`module.go` (package `tsdoc`) scans Go source for specially-tagged `//` comments
and emits a TypeScript declaration file (`.d.ts`). It lets you keep the type
definitions for a JavaScript runtime surface (globals, modules, classes) right
next to the Go code that implements them, instead of hand-maintaining a separate
`.d.ts`.

The public API is two functions:

- `Load(pkg string, debug bool) ([]*Namespace, error)` — parses every `@ts`
  comment block found in the named Go package (and its dependencies).
- `WriteDTS(ns []*Namespace, include map[string]bool) string` — renders the
  selected namespaces to a `.d.ts` string. A namespace is rendered only when
  `include[name] == true`; empty namespaces are skipped.

A typical generator binary loads a package, builds the include map from CLI
flags (e.g. `-i global`, `-e global`), and writes the result to a file.

---

## How comments are read

Two rules decide what the parser sees.

1. **Only comment groups whose first line starts with `@ts ` are considered.**
   A leading ordinary doc-comment on a function is ignored unless it begins with
   `@ts `.

2. **A "block" is one Go comment group.** Go groups consecutive `//` lines into
   one comment group; a blank line (or a line of code) ends the group. Inside a
   block, the first `@ts …` line is the *header* and every following `@`-line is
   a *tag*. Lines without an `@` continue the previous tag (so docs and
   signatures may span multiple lines).

```go
// @ts function md.toHtml          <- header (starts the block)
// @doc Convert markdown to HTML.  <- tag
// continued doc line              <- continues @doc
// @param src - string - Input.    <- tag
```

Because grouping is by blank line, put a blank comment line (`//`) between tags
if you want them visually separated but still in the same block — an *actual*
blank line would start a new block.

---

## The header: `@ts <kind> <identifier>`

Every block starts with `@ts`, a *kind*, and a dotted *identifier*. The
identifier has one to three parts:

| Parts | Meaning | Example |
|-------|---------|---------|
| `namespace` | A namespace/module | `global`, `md`, `buffer` |
| `namespace.Name` | A type living in a namespace | `md.Parser`, `global.Buffer` |
| `namespace.Class.member` | A member of a class | `md.Parser.render` |

The special namespace name `global` renders as `declare global { … }`; any other
name renders as `declare module "name" { … }`.

---

## The `@doc` tag (every kind)

`@doc` is valid on **every** kind below and always does the same thing: it
becomes a `/** … */` JSDoc comment above the declaration. It may span multiple
lines. It is omitted from the per-kind tables that follow to avoid repetition —
assume it is always available.

```go
// @doc
// Convert a markdown string to HTML.
// Supports CommonMark plus tables.
```

---

## Supported kinds

| Kind | Identifier shape | Renders as |
|------|------------------|------------|
| `namespace` | `ns` | `declare global {}` / `declare module "ns" {}` |
| `interface` | `ns.Name` | `interface Name { … }` |
| `type` | `ns.Name` (may include generics, e.g. `ns.Box<T>`) | `type Name = …;` |
| `class` | `ns.Name` | `export class Name { … }` |
| `function` | `ns.name` | `export function name(…): T` |
| `var` / `let` / `const` | `ns.name` | `var name: T` (or `export var …` outside `global`) |
| `method` | `ns.Class.name` | `name(…): T` inside the class body |
| `constructor` | `ns.Class.name` | `constructor(…)` inside the class body |
| `field` | `ns.Class.name` | `name: T` inside the class body |

`method`, `constructor`, and `field` blocks attach to a `class` block with the
same `ns.Class` name. **Declare the class before its members** (the binder only
looks at classes already seen).

The subsections below follow this table's order. Each lists the tags that kind
accepts (besides the universal `@doc`) and shows a focused example.

---

### `namespace`

| Tag | Effect |
|-----|--------|
| `@export <Name> …` | Emits `export { Name };` inside the module. Multiple names allowed, comma- or space-separated, or one per line. Use this to re-export a symbol whose actual type lives in `global`. |

```go
// @ts namespace buffer
// @doc The node:buffer module re-exports Buffer from the global scope.
// @export Buffer
// @export BufferEncoding
```

```typescript
declare module "buffer" {
  export { Buffer };
  export { BufferEncoding };
}
```

---

### `interface`

| Tag | Effect |
|-----|--------|
| `@extends <Base> …` | Interface inheritance (`interface X extends Base`). |
| `@field …` | One member. Two forms accepted (see [The `@field` line](#the-field-line--two-forms)). One `@field` per member. |

Interfaces with the same `ns.Name` declared in different blocks/files **merge**
(standard TypeScript declaration merging) — useful for splitting a large
interface into themed chunks.

```go
// @ts interface md.Config
// @doc Top-level options object.
// @extends BaseConfig
// @field parser? - ParserOptions - Parser extension flags.
// @field strict?: boolean - Fail on the first error.
```

```typescript
/**
 * Top-level options object.
 */
interface Config extends BaseConfig {
  /**
   * Parser extension flags.
   */
  parser?: ParserOptions
  /**
   * Fail on the first error.
   */
  strict?: boolean
}
```

---

### `type`

| Tag | Effect |
|-----|--------|
| `@pred <expr>` | The right-hand side of the alias. With no `@field`, renders `type Name = <expr>;`. With `@field` lines, `<expr>` becomes a prefix before an object literal (e.g. an intersection). |
| `@field …` | An object-literal member; same syntax as interface fields. |

```go
// @ts type global.BufferEncoding
// @pred "utf8" | "utf-8" | "base64" | "hex"
```

```typescript
type BufferEncoding = "utf8" | "utf-8" | "base64" | "hex";
```

Generics go in the identifier; combine `@pred` with `@field` for an
intersection:

```go
// @ts type demo.Tagged<T>
// @doc A value carrying a string tag.
// @pred T &
// @field tag: string - Discriminator.
```

```typescript
type Tagged<T> = T & {
  tag: string
};
```

---

### `class`

| Tag | Effect |
|-----|--------|
| `@default` | Emit `export default class` instead of `export class`. |

A class block only declares the shell and its doc. Members are added with
separate `method` / `constructor` / `field` blocks (each may carry `@static` /
`@readonly`). Declare the class *before* those member blocks.

```go
// @ts class demo.Greeter
// @doc Builds greetings.
```

```typescript
/**
 * Builds greetings.
 */
export class Greeter {
}
```

---

### `function`

| Tag | Effect |
|-----|--------|
| `@param <name> [- <type>] [- <desc>]` | One parameter. Type defaults to `any`. Optional params use a trailing `?` (`opts?`). One line per parameter. |
| `@returns [<type>] [- <desc>]` | Return type and/or description. Type defaults to `void` when empty, `any` when only a description is given. |
| `@example` | Example code, placed in the JSDoc as `@example`. |
| `@sig <signature>` | Override: use this exact signature verbatim instead of generating one from `@param`/`@returns`. |
| `@default` | Emit `export default` instead of `export`. |

```go
// @ts function md.toHtml
// @doc Convert markdown to HTML.
// @param src - string - The markdown source.
// @param opts? - Config - Optional configuration.
// @returns string - The rendered HTML.
```

```typescript
/**
 * Convert markdown to HTML.
 * @param src The markdown source.
 * @param opts Optional configuration.
 * @returns The rendered HTML.
 */
export function toHtml (src: string, opts?: Config): string
```

Use `@sig` when the signature is too complex to generate (overloads, generics,
rest params, type predicates):

```go
// @ts function dom.isElement
// @sig function isElement(node: Node): node is Element
// @doc Type-guard for DOM elements.
```

---

### `var` / `let` / `const`

| Tag | Effect |
|-----|--------|
| `@type <T>` | The variable's type. Renders `var name: T`. Inside `global` the `export` keyword is omitted (global vars are auto-exported); elsewhere it becomes `export var name: T`. |

```go
// @ts var global.Buffer
// @type BufferConstructor
// @doc The global Buffer constructor.
```

```typescript
/**
 * The global Buffer constructor.
 */
var Buffer: BufferConstructor
```

---

### `method`

| Tag | Effect |
|-----|--------|
| `@param` / `@returns` / `@example` / `@sig` | Same as `function`. |
| `@static` | Prepend `static `. |

Attaches to the `class` of the same `ns.Class` name. Example (note: no `@sig`,
so the signature is generated from `@param`/`@returns` and `@static` adds the
modifier):

```go
// @ts class demo.Greeter
// @doc Builds greetings.

// @ts method demo.Greeter.fromLocale
// @static
// @doc Construct a greeter for a locale.
// @param locale - string - BCP-47 locale tag.
// @returns Greeter - A configured greeter.
```

```typescript
export class Greeter {
  /**
   * Construct a greeter for a locale.
   * @param locale BCP-47 locale tag.
   * @returns A configured greeter.
   */
  static fromLocale(locale: string): Greeter
}
```

---

### `constructor`

| Tag | Effect |
|-----|--------|
| `@param` / `@sig` | Same as `function`. (`@static` is ignored — constructors can't be static.) |

```go
// @ts constructor demo.Greeter.new
// @doc Create a greeter.
// @param options? - Options - Optional configuration.
```

```typescript
constructor (options?: Options)
```

---

### `field`

| Tag | Effect |
|-----|--------|
| `@type <T>` | The field's type (renders `name: T`). |
| `@static` | Prepend `static `. |
| `@readonly` | Prepend `readonly `. |

Attaches to the `class` of the same `ns.Class` name.

```go
// @ts field demo.Greeter.locale
// @readonly
// @type string
// @doc The locale this greeter was created with.
```

```typescript
/**
 * The locale this greeter was created with.
 */
readonly locale: string
```

---

## The `@field` line — two forms

`@field` (used by `interface` and `type`) accepts either an already-complete
signature or a name/type/description triple. The parser decides based on
whether the part before the first ` - ` contains a `:` or `(`.

**Form 1 — full signature** (contains `:` or `(`):

```go
// @field write(s: string, encoding?: BufferEncoding): number - Writes s.
// @field compare(a: Uint8Array, b: Uint8Array): -1 | 0 | 1 - Compares a to b.
// @field poolSize: number - Pool size in bytes.
```

**Form 2 — name / type / desc** (no `:` or `(` before the first ` - `):

```go
// @field startTag? - string - Opening delimiter. Default "{{".
//   ->  startTag?: string
// @field parser? - ParserOptions - Parser extension flags.
//   ->  parser?: ParserOptions
```

A bare name with no type (`@field foo`) renders as `foo: any`.

> **Separator rule:** the description (and type/sig) separator is always `" - "`
> — a dash **with spaces on both sides**. This is what lets a signature contain
> a bare dash, such as the literal type `-1 | 0 | 1` or an arrow type, without
> being chopped. Inside descriptions you may use dashes freely.

---

## Modifiers cheat-sheet

| Tag | Valid on | Output |
|-----|----------|--------|
| `@default` | `function`, `class` | `export default …` |
| `@static` | `method`, `field` | `static …` |
| `@readonly` | `field` | `readonly …` |
| `@extends` | `interface` | `extends Base` |
| `@export` | `namespace` | `export { Name };` |

---

## Rendering rules worth knowing

- **`global` vs modules.** `global` → `declare global { … }` with `export {};`
  on top. Top-level `var`/`function` inside `global` are emitted **without**
  `export` (they're already global); inside a `declare module` they keep
  `export`.
- **Namespace merging.** The same namespace name across multiple files is merged
  into one rendered block. The first block's `@doc` wins; declarations and
  `@export`s accumulate.
- **Empty namespaces are skipped.** A `@ts namespace x` with no interfaces,
  classes, types, vars, or `@export`s produces no output (no empty
  `declare module "x" {}` shell).
- **Selection.** Only namespaces with `include[name] == true` in the `WriteDTS`
  map are written.

---

## Full example

Go source (`example.go`):

```go
package js

// @ts namespace demo
// @doc A small demonstration module.
// @export Greeter

// @ts interface demo.Options
// @doc Options for the greeter.
// @field prefix? - string - Text placed before the name. Default "Hello".
// @field loud? - boolean - Uppercase the result.

// @ts class demo.Greeter
// @doc Builds greetings from a name and Options.

// @ts constructor demo.Greeter.new
// @doc Create a greeter with optional configuration.
// @param options? - Options - Optional configuration.

// @ts field demo.Greeter.locale
// @readonly
// @type string
// @doc The locale this greeter was created with.

// @ts method demo.Greeter.greet
// @doc Produce a greeting for the given name.
// @param name - string - Who to greet.
// @returns string - The greeting.

// @ts method demo.Greeter.fromLocale
// @static
// @doc Build a greeter for a locale.
// @param locale - string - BCP-47 locale tag.
// @returns Greeter - A configured greeter.
func wire() { /* runtime wiring elided */ }
```

Generated `.d.ts` (rendering namespace `demo`):

```typescript
/**
 * A small demonstration module.
 */
declare module "demo" {
  /**
   * Options for the greeter.
   */
  interface Options {
    /**
     * Text placed before the name. Default "Hello".
     */
    prefix?: string
    /**
     * Uppercase the result.
     */
    loud?: boolean
  }
  /**
   * Builds greetings from a name and Options.
   */
  export class Greeter {
    /**
     * Create a greeter with optional configuration.
     * @param options Optional configuration.
     */
    constructor (options?: Options)
    /**
     * The locale this greeter was created with.
     */
    readonly locale: string
    /**
     * Produce a greeting for the given name.
     * @param name Who to greet.
     * @returns The greeting.
     */
    greet(name: string): string
    /**
     * Build a greeter for a locale.
     * @param locale BCP-47 locale tag.
     * @returns A configured greeter.
     */
    static fromLocale(locale: string): Greeter
  }
  export { Greeter };
}
```
