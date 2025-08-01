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
				}
				for _, ty := range ns.Types {
					parseType(ty)
				}
			}

			namespaces = append(namespaces, localNs...)

			if printDebug {
				debug(localNs)
			}
		}
	}
	return namespaces, nil
}

func WriteDTS(ns []*Namespace, inc map[string]bool) string {
	var out strings.Builder

	for _, n := range ns {
		if include, ok := inc[n.Name]; ok && include {
			n.Render(&out, "")
		}
	}

	return out.String()
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
type TsEntry struct {
	Kind       string // constructor, function, method
	Header     string // the header e.g. @ts function tpl
	RawContent []string
	DocLines   []string

	IsDefault bool
	Params    []ParamType
	Returns   *ReturnType
	Example   string
	Signature string
}

// @ts interface md.ParserOptions
type TsInterface struct {
	Name       string
	RawContent []string
	DocLines   []string

	Extends string
	Fields  []FieldType
}

// @ts class md.Parser
type TsClass struct {
	Name       string
	RawContent []string
	DocLines   []string

	Methods   []TsEntry
	IsDefault bool
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
			ns.DocLines = block.Content

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
		case "method":
			for _, cls := range ns.Classes {
				if cls.Name == id.Class {
					cls.Methods = append(cls.Methods, TsEntry{
						Kind:       "method",
						Header:     block.TagLine,
						RawContent: block.Content,
					})
					break
				}
			}
		case "constructor":
			for _, cls := range ns.Classes {
				if cls.Name == id.Class {
					cls.Methods = append(cls.Methods, TsEntry{
						Kind:       "constructor",
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
		case "@type":
			if entry.Kind == "var" || entry.Kind == "let" || entry.Kind == "const" {
				t := strings.Join(buffer, " ")
				entry.Signature = fmt.Sprintf("%s %s: %s", entry.Kind, getNameFromHeader(entry.Header), t)
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

func parseClass(cls *TsClass) {
	var currentTag string
	var buffer []string

	flush := func() {
		switch currentTag {
		case "@default":
			cls.IsDefault = true
		case "@doc":
			cls.DocLines = append(cls.DocLines, buffer...)
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

// @method log(...data: any[]): void; - Logs the provided arguments to the console with info level.
func parseFieldLine(line string) FieldType {
	parts := strings.SplitN(line, "-", 2)
	sigPart := strings.TrimSpace(parts[0])

	desc := ""
	if len(parts) == 2 {
		desc = strings.TrimSpace(parts[1])
	}

	return FieldType{
		Signature: sigPart,
		Desc:      desc,
	}
}

// @param markdown - string - The Markdown content.
func parseParamLine(line string) ParamType {
	parts := strings.SplitN(line, "-", 3)

	if len(parts) < 2 {
		fmt.Printf("Warning: param line malformed: %q\n", line)
		return ParamType{}
	}

	name := strings.TrimSpace(parts[0])
	name = strings.Trim(name, `"`)
	typ := "any"
	desc := ""

	if len(parts) == 2 {
		desc = strings.TrimSpace(parts[1])
	} else {
		typ = strings.TrimSpace(parts[1])
		desc = strings.TrimSpace(parts[2])
	}

	return ParamType{
		Name: name,
		Type: typ,
		Desc: desc,
	}
}

// similar to parseParamLine, but return values have no name
// @returns Promise<string> - The rendered HTML string.
func parseReturnLine(line string) *ReturnType {
	if line == "" {
		return &ReturnType{
			Type: "void",
			Desc: "",
		}
	}

	parts := strings.SplitN(line, "-", 2)

	if len(parts) < 1 {
		fmt.Printf("Warning: return line malformed: %q\n", line)
		return nil
	}

	typ := "any"
	desc := ""
	if len(parts) == 1 {
		desc = strings.TrimSpace(parts[0])
	} else {
		typ = strings.Trim(strings.TrimSpace(parts[0]), `"`)
		desc = strings.TrimSpace(parts[1])
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
		entry.Render(out, indent+"  ")
	}

	for _, cls := range ns.Classes {
		cls.Render(out, indent+"  ")
	}
	for _, t := range ns.Types {
		t.Render(out, indent+"  ")
	}

	fmt.Fprintf(out, "%s}\n", indent)

}

func (e *TsEntry) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, e.DocLines, e.Params, e.Returns, e.Example)

	if e.Signature != "" {
		if e.IsDefault {
			out.WriteString(indent + "export default " + trimExportKeyword(e.Signature) + "\n")
		} else {
			if e.Kind == "constructor" || e.Kind == "method" || e.Kind == "field" {
				out.WriteString(indent + trimExportKeyword(e.Signature) + "\n")
			} else {
				out.WriteString(indent + "export " + trimExportKeyword(e.Signature) + "\n")
			}
		}
	}
}

func (cls *TsClass) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, cls.DocLines, nil, nil, "")

	if cls.IsDefault {
		fmt.Fprintf(out, "%sexport default class %s{\n", indent, cls.Name)
	} else {
		fmt.Fprintf(out, "%sexport class %s{\n", indent, cls.Name)
	}

	for _, method := range cls.Methods {
		method.Render(out, indent+"  ")
	}

	fmt.Fprintf(out, "%s}\n", indent)
}

func (int *TsInterface) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, int.DocLines, nil, nil, "")

	if int.Extends != "" {
		fmt.Fprintf(out, "%s interface %s extends %s", indent, int.Name, int.Extends)
	} else {
		fmt.Fprintf(out, "%sinterface %s", indent, int.Name)
	}

	if len(int.Fields) > 0 {
		fmt.Fprintf(out, " {\n")
		for _, method := range int.Fields {
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
