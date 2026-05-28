package tsdoc

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Load loads TypeScript documentation from Go source files.
//
// @ts namespace md
// @doc This is the top‑level module.

// @ts class md.Parser
// @doc A class called Parser in namespace md
// It will be exported as default
// @default

// @ts method md.Parser.render
// @doc A method called render
// @param options - RenderOptions - First argument called options of type RenderOptions
// @returns void - Return type
// @example
//
//	const p = new Parser();
//	p.render({ color: "red" });
func Load(name string, printDebug bool) ([]*Namespace, error) {
	cfg := &packages.Config{Mode: packages.NeedSyntax | packages.NeedFiles | packages.NeedName | packages.NeedDeps}
	pkgs, err := packages.Load(cfg, name)
	if err != nil {
		return nil, err
	}

	var namespaces []*Namespace
	nsByName := make(map[string]*Namespace)

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			var comments []*ast.Comment

			for _, c := range file.Comments {
				line := strings.TrimPrefix(c.Text(), "//")

				if strings.HasPrefix(line, "@ts ") {
					comments = append(comments, c.List...)
				}
			}

			blocks := parseCommentBlocks(comments)
			if printDebug {
				fmt.Printf("Found %d blocks in file %s\n", len(blocks), file.Name)
			}

			localNs := groupNamespaces(blocks)

			for _, ns := range localNs {
				for i := range ns.Entries {
					parseEntry(&ns.Entries[i])
				}
				for _, cls := range ns.Classes {
					parseClass(cls)
					for i := range cls.Methods {
						parseEntry(&cls.Methods[i])
					}
				}
				for _, inter := range ns.Interfaces {
					parseInterface(inter)
					for i := range inter.Methods {
						parseEntry(&inter.Methods[i])
					}
				}
				for _, ty := range ns.Types {
					parseType(ty)
				}
			}

			for _, ns := range localNs {
				if existing, ok := nsByName[ns.Name]; ok {
					mergeNamespace(existing, ns)
				} else {
					nsByName[ns.Name] = ns
					namespaces = append(namespaces, ns)
				}
			}

			if printDebug {
				debug(localNs)
			}
		}
	}
	return namespaces, nil
}

func mergeNamespace(dst, src *Namespace) {
	if len(dst.DocLines) == 0 {
		dst.DocLines = src.DocLines
	}
	dst.Entries = append(dst.Entries, src.Entries...)
	dst.Classes = append(dst.Classes, src.Classes...)
	dst.Interfaces = append(dst.Interfaces, src.Interfaces...)
	dst.Types = append(dst.Types, src.Types...)
	dst.Reexports = append(dst.Reexports, src.Reexports...)
}

func WriteDTS(ns []*Namespace, inc map[string]bool) string {
	var out strings.Builder

	for _, n := range ns {
		if include, ok := inc[n.Name]; !ok || !include {
			continue
		}
		// skip namespaces that have no declarations of their own. The
		if n.isEmpty() {
			continue
		}
		n.Render(&out, "")
	}

	return out.String()
}

// isEmpty reports whether the namespace contains anything renderable.
func (ns *Namespace) isEmpty() bool {
	return len(ns.Entries) == 0 &&
		len(ns.Classes) == 0 &&
		len(ns.Interfaces) == 0 &&
		len(ns.Types) == 0 &&
		len(ns.Reexports) == 0
}

func debug(ns []*Namespace) {
	for _, ns := range ns {
		fmt.Printf("Namespace: %s\n", ns.Name)

		for _, cls := range ns.Interfaces {
			fmt.Printf("  Interface: %s\n", cls.Name)
			if len(cls.DocLines) > 0 {
				fmt.Printf("    Doc: %s\n", strings.Join(cls.DocLines, " "))
			}
			for _, method := range cls.Fields {
				fmt.Printf("    Fields: %s\n", method.Signature)
				fmt.Printf("      Doc: %s\n", method.Desc)
			}
		}

		for _, t := range ns.Types {
			fmt.Printf("  Type: %s\n", t.Name)
			if len(t.DocLines) > 0 {
				fmt.Printf("    Doc: %s\n", strings.Join(t.DocLines, " "))
			}
			fmt.Printf("    Definition: %s\n", t.Definition)
			fmt.Printf("    Pred: %s\n", t.Pred)
			for _, f := range t.Fields {
				fmt.Printf("    Field: %s\n", f.Signature)
				fmt.Printf("      Doc: %s\n", f.Desc)
			}
		}

		for _, entry := range ns.Entries {
			fmt.Printf("  [%s] %s\n", entry.Kind, entry.Header)
			if len(entry.DocLines) > 0 {
				fmt.Printf("    Doc: %s\n", strings.Join(entry.DocLines, " "))
			}
			for _, p := range entry.Params {
				fmt.Printf("    Param: %q (%s): %s\n", p.Name, p.Type, p.Desc)
			}
			fmt.Printf("    Returns: %s: %s\n", entry.Returns.Type, entry.Returns.Desc)
			if entry.Signature != "" {
				fmt.Printf("    Signature: %s\n", entry.Signature)
			}
			if entry.Example != "" {
				fmt.Printf("    Example:\n%s\n", indent(entry.Example, "      "))
			}
		}

		for _, cls := range ns.Classes {
			fmt.Printf("  Class: %s\n", cls.Name)
			if len(cls.DocLines) > 0 {
				fmt.Printf("    Doc: %s\n", strings.Join(cls.DocLines, " "))
			}
			for _, method := range cls.Methods {
				fmt.Printf("    Method: %s\n", method.Header)
				if len(method.DocLines) > 0 {
					fmt.Printf("      Doc: %s\n", strings.Join(method.DocLines, " "))
				}
				for _, p := range method.Params {
					fmt.Printf("      Param: %s %s - %s\n", p.Name, p.Type, p.Desc)
				}
				fmt.Printf("      Returns: %s: %s\n", method.Returns.Type, method.Returns.Desc)
				if method.Signature != "" {
					fmt.Printf("      Signature: %s\n", method.Signature)
				}
				if method.Example != "" {
					fmt.Printf("      Example:\n%s\n", indent(method.Example, "        "))
				}
			}
		}

	}
	fmt.Printf("\n")
}

func indent(text string, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

type CommentBlock struct {
	TagLine string
	Content []string
}

func parseCommentBlocks(comments []*ast.Comment) []CommentBlock {
	var tags []CommentBlock
	var current *CommentBlock

	for _, c := range comments {
		line := strings.TrimPrefix(c.Text, "//")
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "@ts ") {
			if current != nil {
				tags = append(tags, *current)
			}
			current = &CommentBlock{
				TagLine: line,
				Content: []string{},
			}
		} else if current != nil {
			current.Content = append(current.Content, line)
		}
		// fmt.Printf("+++comment line: %q\n", line)
	}

	if current != nil {
		tags = append(tags, *current)
	}

	return tags
}

// Identifier represents a TypeScript identifier in the format:
// namespace.Class.Member
type Identifier struct {
	Namespace string
	Class     string
	Member    string
}

// @ts namespace md
type Namespace struct {
	Name       string
	DocLines   []string
	Entries    []TsEntry
	Classes    []*TsClass
	Interfaces []*TsInterface
	Types      []*TsType

	// Reexports is a list of bare symbol names to re-export from this
	// namespace, populated from `@export <Name>` lines inside an
	// `@ts namespace <name>` block. Each entry renders as
	// `export { Name };` and is intended for module namespaces that just
	// republish symbols whose actual type definitions live elsewhere
	// (typically in `global`).
	Reexports []string
}

// @param markdown - string - The Markdown content.
type ParamType struct {
	Name string
	Type string
	Desc string
}

// @field NoIntraEmphasis?: boolean - Ignore emphasis markers inside words
type FieldType struct {
	Signature string
	Desc      string
	InDoc     bool
}

// @returns Promise<string> - The rendered HTML string.
type ReturnType struct {
	Type string
	Desc string
}

// @ts function md.toHtml
// @ts var global.console
// @ts const md.commonOptions
// @ts method md.Parser.render
// @ts field md.Parser.poolSize
type TsEntry struct {
	Kind       string // constructor, function, method, field, var, let, const
	Header     string // the header e.g. @ts function tpl
	RawContent []string
	DocLines   []string

	IsDefault  bool
	IsStatic   bool // emit `static ` prefix; only meaningful on class methods/fields
	IsReadonly bool // emit `readonly ` prefix; only meaningful on class/interface fields
	Params     []ParamType
	Returns    *ReturnType
	Example    string
	Signature  string
}

// @ts interface md.ParserOptions
type TsInterface struct {
	Name       string
	RawContent []string
	DocLines   []string

	Extends string
	Fields  []FieldType
	Methods []TsEntry
}

// @ts class md.Parser
type TsClass struct {
	Name       string
	RawContent []string
	DocLines   []string

	Methods    []TsEntry
	IsDefault  bool
	Implements []string
}

// @ts type global.IndexContext
type TsType struct {
	Name       string
	RawContent []string
	DocLines   []string

	Fields     []FieldType
	Definition string
	Pred       string
}

// TODO TsType, TsEnum
// export type UserID = string | number;
// export type User = { id: UserID; name: string };

func parseIdentifier(id string) (*Identifier, error) {
	parts := strings.Split(id, ".")

	switch len(parts) {
	case 1:
		return &Identifier{Namespace: parts[0]}, nil
	case 2:
		return &Identifier{Namespace: parts[0], Class: parts[1]}, nil
	case 3:
		return &Identifier{Namespace: parts[0], Class: parts[1], Member: parts[2]}, nil
	default:
		return nil, fmt.Errorf("invalid identifier: %s", id)
	}
}

func groupNamespaces(blocks []CommentBlock) []*Namespace {
	var namespaces []*Namespace
	nsMap := make(map[string]*Namespace)

	for _, block := range blocks {
		fields := strings.Fields(block.TagLine)

		if len(fields) < 3 || fields[0] != "@ts" {
			continue
		}

		tagType := fields[1]
		id, err := parseIdentifier(fields[2])
		if err != nil {
			fmt.Printf("Warning: skipping invalid identifier %q\n", fields[2])
			continue
		}

		ns, ok := nsMap[id.Namespace]
		if !ok {
			ns = &Namespace{
				Name:       id.Namespace,
				DocLines:   []string{},
				Entries:    []TsEntry{},
				Classes:    []*TsClass{},
				Interfaces: []*TsInterface{},
			}
			nsMap[id.Namespace] = ns
			namespaces = append(namespaces, ns)
		}

		switch tagType {
		case "namespace":
			doc, exports := parseNamespaceBlock(block.Content)
			ns.DocLines = append(ns.DocLines, doc...)
			ns.Reexports = append(ns.Reexports, exports...)

		case "interface":
			inter := &TsInterface{
				Name:       id.Class,
				RawContent: block.Content,
				Fields:     []FieldType{},
			}
			ns.Interfaces = append(ns.Interfaces, inter)
		case "class":
			class := &TsClass{
				Name:       id.Class,
				RawContent: block.Content,
				Methods:    []TsEntry{},
			}
			ns.Classes = append(ns.Classes, class)
		case "method", "constructor", "field":
			for _, cls := range ns.Classes {
				if cls.Name == id.Class {
					cls.Methods = append(cls.Methods, TsEntry{
						Kind:       tagType,
						Header:     block.TagLine,
						RawContent: block.Content,
					})
					break
				}
			}
			for _, inter := range ns.Interfaces {
				if inter.Name == id.Class {
					inter.Methods = append(inter.Methods, TsEntry{
						Kind:       tagType,
						Header:     block.TagLine,
						RawContent: block.Content,
					})
					break
				}
			}
		case "type":
			t := &TsType{
				Name:       id.Class,
				RawContent: block.Content,
				Fields:     []FieldType{},
			}
			ns.Types = append(ns.Types, t)
		default:
			ns.Entries = append(ns.Entries, TsEntry{
				Kind:       tagType,
				Header:     block.TagLine,
				RawContent: block.Content,
			})
		}
	}

	return namespaces
}

func parseEntry(entry *TsEntry) {
	var currentTag string
	var buffer []string

	flush := func() {
		switch currentTag {
		case "@doc":
			entry.DocLines = append(entry.DocLines, buffer...)
		case "@param":
			for _, line := range buffer {
				param := parseParamLine(line)
				entry.Params = append(entry.Params, param)
			}
		case "@returns":
			entry.Returns = parseReturnLine(strings.Join(buffer, " "))
		case "@example":
			entry.Example = strings.Join(buffer, "\n")
		case "@sig":
			entry.Signature = strings.Join(buffer, " ")
		case "@default":
			if entry.Kind == "function" {
				// constructor or method can't be default
				entry.IsDefault = true
			}
		case "@static":
			entry.IsStatic = true
		case "@readonly":
			entry.IsReadonly = true
		case "@type":
			t := strings.Join(buffer, " ")
			name := getNameFromHeader(entry.Header)
			switch entry.Kind {
			case "var", "let", "const":
				entry.Signature = fmt.Sprintf("%s %s: %s", entry.Kind, name, t)
			case "field":
				entry.Signature = fmt.Sprintf("%s: %s", name, t)
			}
		}
		buffer = nil
	}

	for _, line := range entry.RawContent {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@") {
			flush()
			currentTag = firstWord(line)
			buffer = []string{strings.TrimSpace(line[len(currentTag):])}
		} else if currentTag != "" {
			buffer = append(buffer, line)
		}
	}
	flush()

	if (entry.Kind == "function" || entry.Kind == "method" || entry.Kind == "constructor") && entry.Signature == "" {
		entry.Signature = generateSignature(entry)
	}

	if entry.Returns == nil {
		entry.Returns = parseReturnLine("")
	}
}

// parseNamespaceBlock walks the raw content of an `@ts namespace X` block and
// extracts the prose (`@doc`) and the list of bare symbols to re-export
// (`@export Foo`). Lines without a recognised tag are inherited by the most
// recent tag, mirroring the behaviour of parseInterface / parseClass.
//
// Example block:
//
//	@ts namespace buffer
//	@doc The node:buffer module re-exports Buffer from the global scope.
//	@export Buffer
//	@export BufferEncoding
func parseNamespaceBlock(content []string) (doc, exports []string) {
	var currentTag string
	var buffer []string

	flush := func() {
		switch currentTag {
		case "@doc":
			doc = append(doc, buffer...)
		case "@export":
			for _, raw := range buffer {
				for _, tok := range strings.FieldsFunc(raw, func(r rune) bool {
					return r == ',' || r == ' ' || r == '\t'
				}) {
					if tok != "" {
						exports = append(exports, tok)
					}
				}
			}
		}
		buffer = nil
	}

	for _, line := range content {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@") {
			flush()
			currentTag = firstWord(line)
			buffer = []string{strings.TrimSpace(line[len(currentTag):])}
		} else if currentTag != "" {
			buffer = append(buffer, line)
		}
	}
	flush()
	return doc, exports
}

func parseClass(cls *TsClass) {
	var currentTag string
	var buffer []string

	flush := func() {
		switch currentTag {
		case "@default":
			cls.IsDefault = true
		case "@doc":
			cls.DocLines = append(cls.DocLines, buffer...)
		case "@implements":
			cls.Implements = append(cls.Implements, strings.Join(buffer, " "))
		}
		buffer = nil
	}

	for _, line := range cls.RawContent {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@") {
			flush()
			currentTag = firstWord(line)
			buffer = []string{strings.TrimSpace(line[len(currentTag):])}
		} else if currentTag != "" {
			buffer = append(buffer, line)
		}
	}
	flush()
}

func parseInterface(inter *TsInterface) {
	var currentTag string
	var buffer []string

	flush := func() {
		switch currentTag {
		case "@doc":
			inter.DocLines = append(inter.DocLines, buffer...)
		case "@extends":
			inter.Extends = strings.Join(buffer, " ")
		case "@field":
			field := parseFieldLine(strings.Join(buffer, " "))
			inter.Fields = append(inter.Fields, field)
		}
		buffer = nil
	}

	for _, line := range inter.RawContent {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@") {
			flush()
			currentTag = firstWord(line)
			buffer = []string{strings.TrimSpace(line[len(currentTag):])}
		} else if currentTag != "" {
			buffer = append(buffer, line)
		}
	}
	flush()
}

// TODO parse all documentation equally
// also, take into account multiple lines
func parseType(t *TsType) {
	var currentTag string
	var buffer []string
	flush := func() {
		switch currentTag {
		case "@doc":
			t.DocLines = append(t.DocLines, buffer...)
		case "@pred":
			t.Pred = strings.Join(buffer, " ")
		case "@field":
			field := parseFieldLine(strings.Join(buffer, " "))

			t.Fields = append(t.Fields, field)
		}
		buffer = nil
	}
	for _, line := range t.RawContent {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@") {
			flush()
			currentTag = firstWord(line)
			buffer = []string{strings.TrimSpace(line[len(currentTag):])}
		} else if currentTag != "" {
			buffer = append(buffer, line)
		}
	}
	flush()
}

func generateSignature(entry *TsEntry) string {
	name := getNameFromHeader(entry.Header)
	if name == "" {
		return ""
	}

	params := make([]string, len(entry.Params))
	for i, p := range entry.Params {
		params[i] = fmt.Sprintf("%s: %s", p.Name, p.Type)
	}

	ret := entry.Returns

	switch entry.Kind {
	case "constructor":
		return fmt.Sprintf("constructor (%s)", strings.Join(params, ", "))
	case "function":
		return fmt.Sprintf("function %s (%s): %s", name, strings.Join(params, ", "), ret.Type)
	case "method":
		return fmt.Sprintf("%s(%s): %s", name, strings.Join(params, ", "), ret.Type)
	default:
		return ""
	}
}

func getNameFromHeader(header string) string {
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return ""
	}

	parts := strings.Split(fields[2], ".")
	return parts[len(parts)-1]
}

func firstWord(line string) string {
	fields := strings.Fields(line)
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func trimExportKeyword(sig string) string {
	sig = strings.TrimSpace(sig)
	sig = strings.TrimPrefix(sig, "export ")
	return sig
}

// dashSep is the separator between a signature/type and its trailing
// description in @field, @param and @returns lines.
const dashSep = " - "

// splitSignatureDoc splits "signature - description" once on the first " - ".
// Both parts are trimmed. Either part may be empty.
func splitSignatureDoc(line string) (sig, desc string) {
	line = strings.TrimSpace(line)
	if i := strings.Index(line, dashSep); i >= 0 {
		return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+len(dashSep):])
	}
	return line, ""
}

// splitDashFields splits line on " - " into at most n trimmed fields.
func splitDashFields(line string, n int) []string {
	parts := strings.SplitN(strings.TrimSpace(line), dashSep, n)
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// parseFieldLine parses a single @field line into a TypeScript member
// signature and an optional description. Two authoring forms are supported:
//
//  1. Full-signature form — the part before the first " - " is already a
//     complete TS member signature:
//
//     @field log(...data: any[]): void - Logs the provided arguments.
//     @field compare(a: Uint8Array, b: Uint8Array): -1 | 0 | 1 - Compares a to b.
//     @field poolSize: number - Pool size in bytes.
//
//  2. Name/type/desc form — like @param, three " - "-separated fields. The
//     middle field is the type; we synthesise `name: type`:
//
//     @field parser? - ParserOptions - Controls parser extensions.
//     -> parser?: ParserOptions
func parseFieldLine(line string) FieldType {
	parts := splitDashFields(line, 3)

	lead := ""
	if len(parts) > 0 {
		lead = parts[0]
	}

	// Form (1):
	if strings.ContainsAny(lead, ":(") {
		desc := ""
		if i := strings.Index(line, dashSep); i >= 0 {
			desc = strings.TrimSpace(line[i+len(dashSep):])
		}
		return FieldType{Signature: lead, Desc: desc}
	}

	// Form (2):
	name := lead
	typ := "any"
	desc := ""
	if len(parts) < 3 {
		typ = parts[1]
	} else {
		typ = parts[1]
		desc = parts[2]
	}

	return FieldType{
		Signature: buildFieldSignature(name, typ),
		Desc:      desc,
	}
}

// buildFieldSignature turns a bare name (optionally ending in '?') and a type
// into a `name: type` / `name?: type` member signature.
func buildFieldSignature(name, typ string) string {
	name = strings.TrimSpace(name)
	typ = strings.TrimSpace(typ)
	if typ == "" {
		typ = "any"
	}
	return name + ": " + typ
}

// parseParamLine parses a single @param line into name/type/description, e.g.
//
//	@param markdown - string - The Markdown content.
//
// Like parseFieldLine, the separator is " - " (space-dash-space) so the type
// portion can safely contain bare dashes (`-1`, `() => Foo<-T>` etc.).
// Accepted forms:
//
//	@param name                       -> name="name", type="any",     desc=""
//	@param name - desc                -> name="name", type="any",     desc="desc"
//	@param name - type - desc         -> name="name", type="type",    desc="desc"
func parseParamLine(line string) ParamType {
	parts := splitDashFields(line, 3)

	if len(parts) == 0 || parts[0] == "" {
		fmt.Printf("Warning: param line malformed: %q\n", line)
		return ParamType{}
	}

	name := strings.Trim(parts[0], `"`)
	typ := "any"
	desc := ""

	if len(parts) < 3 {
		desc = parts[1]
	} else {
		typ = strings.Trim(parts[1], `"`)
		desc = parts[2]
	}

	return ParamType{
		Name: name,
		Type: typ,
		Desc: desc,
	}
}

// parseReturnLine parses an @returns line into a type and optional description.
//
//	@returns void
//	@returns Promise<string> - The rendered HTML string.
//	@returns -1 | 0 | 1 - Sign of the comparison.
func parseReturnLine(line string) *ReturnType {
	if line == "" {
		return &ReturnType{
			Type: "void",
			Desc: "",
		}
	}

	typ, desc := splitSignatureDoc(line)
	typ = strings.Trim(typ, `"`)
	if typ == "" {
		typ = "any"
	}

	return &ReturnType{
		Type: typ,
		Desc: desc,
	}
}

func writeDoc(out *strings.Builder, indent string, docLines []string, params []ParamType, returns *ReturnType, example string) {
	hasDoc := len(docLines) > 0 || example != "" || len(params) > 0 || (returns != nil && returns.Desc != "")

	if hasDoc {
		out.WriteString(indent + "/**\n")
	}

	for _, line := range docLines {
		fmt.Fprintf(out, "%s * %s\n", indent, strings.TrimSpace(line))
	}

	for _, p := range params {
		fmt.Fprintf(out, "%s * @param %s %s\n", indent, p.Name, p.Desc)
	}

	if returns != nil && returns.Desc != "" {
		fmt.Fprintf(out, "%s * @returns %s\n", indent, returns.Desc)
	}

	if hasDoc {
		fmt.Fprintf(out, "%s *\n", indent)
	}

	if example != "" {
		lines := strings.Split(example, "\n")
		for i := range lines {
			lines[i] = strings.TrimSpace(lines[i])
			if lines[i] != "" {
				lines[i] = fmt.Sprintf("%s * %s", indent, lines[i])
			}
		}
		fmt.Fprintf(out, "%s * @example\n", indent)
		fmt.Fprintf(out, "%s * %s\n", indent, strings.Join(lines, "\n"))
	}

	if hasDoc {
		out.WriteString(indent + " */\n")
	}
}

func (ns *Namespace) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, ns.DocLines, nil, nil, "")

	if ns.Name == "global" {
		fmt.Fprintf(out, "export {};\n")
		fmt.Fprintf(out, "%sdeclare %s {\n", indent, ns.Name)
	} else {
		fmt.Fprintf(out, "%sdeclare module \"%s\" {\n", indent, ns.Name)
	}

	for _, inter := range ns.Interfaces {
		inter.Render(out, indent+"  ")
	}

	for _, entry := range ns.Entries {
		entry.render(out, indent+"  ", ns.Name)
	}

	for _, cls := range ns.Classes {
		cls.Render(out, indent+"  ")
	}
	for _, t := range ns.Types {
		t.Render(out, indent+"  ")
	}

	for _, name := range ns.Reexports {
		fmt.Fprintf(out, "%s  export { %s };\n", indent, name)
	}

	fmt.Fprintf(out, "%s}\n", indent)

}

func (e *TsEntry) Render(out *strings.Builder, indent string) {
	e.render(out, indent, "")
}

func (e *TsEntry) render(out *strings.Builder, indent, nsName string) {
	writeDoc(out, indent, e.DocLines, e.Params, e.Returns, e.Example)

	if e.Signature == "" {
		return
	}

	sig := trimExportKeyword(e.Signature)

	var modifiers string
	if isMember(e.Kind) {
		if e.IsStatic && e.Kind != "constructor" {
			modifiers += "static "
		}
		if e.IsReadonly && (e.Kind == "field") {
			modifiers += "readonly "
		}
	}

	switch {
	case e.IsDefault:
		out.WriteString(indent + "export default " + sig + "\n")
	case isMember(e.Kind):
		out.WriteString(indent + modifiers + sig + "\n")
	case nsName == "global":
		out.WriteString(indent + sig + "\n")
	default:
		out.WriteString(indent + "export " + sig + "\n")
	}
}

// isMember reports whether the entry kind names a class/interface member as
// opposed to a top-level declaration.
func isMember(kind string) bool {
	return kind == "constructor" || kind == "method" || kind == "field"
}

func (cls *TsClass) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, cls.DocLines, nil, nil, "")

	keyword := "export class"
	if cls.IsDefault {
		keyword = "export default class"
	}
	implements := ""
	if len(cls.Implements) > 0 {
		implements = " implements " + strings.Join(cls.Implements, ", ")
	}
	fmt.Fprintf(out, "%s%s %s%s {\n", indent, keyword, cls.Name, implements)

	for _, method := range cls.Methods {
		method.Render(out, indent+"  ")
	}

	fmt.Fprintf(out, "%s}\n", indent)
}

func (int *TsInterface) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, int.DocLines, nil, nil, "")

	if int.Extends != "" {
		fmt.Fprintf(out, "%sinterface %s extends %s", indent, int.Name, int.Extends)
	} else {
		fmt.Fprintf(out, "%sinterface %s", indent, int.Name)
	}

	if len(int.Fields) > 0 || len(int.Methods) > 0 {
		fmt.Fprintf(out, " {\n")
		for _, method := range int.Fields {
			method.Render(out, indent+"  ")
		}

		for _, method := range int.Methods {
			method.Render(out, indent+"  ")
		}

		fmt.Fprintf(out, "%s}\n", indent)
	} else {
		fmt.Fprintf(out, " {}\n")
	}
}

func (p FieldType) Render(out *strings.Builder, indent string) {

	if p.Desc != "" {
		out.WriteString(indent + "/**\n")

		fmt.Fprintf(out, "%s * %s\n", indent, p.Desc)
		// TODO: support multiple line comments
		out.WriteString(indent + " */\n")
	}

	fmt.Fprintf(out, "%s%s\n", indent, p.Signature)
}

func (t *TsType) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, t.DocLines, nil, nil, "")
	if len(t.Fields) > 0 {
		prefix := t.Pred
		if prefix != "" {
			prefix += " "
		}

		fmt.Fprintf(out, "%stype %s = %s{\n", indent, t.Name, prefix)

		for _, f := range t.Fields {
			f.Render(out, indent+"  ")
		}

		fmt.Fprintf(out, "%s};\n", indent)
	} else {
		fmt.Fprintf(out, "%stype %s = %s;\n", indent, t.Name, t.Pred)
	}
}
