package tsdoc

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

func Load(name string) ([]*Namespace, error) {
	cfg := &packages.Config{Mode: packages.NeedSyntax | packages.NeedFiles | packages.NeedName | packages.NeedDeps}
	pkgs, err := packages.Load(cfg, name)
	if err != nil {
		return nil, err
	}

	var namespaces []*Namespace
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			var comments []*ast.Comment

			for _, cg := range file.Comments {
				comments = append(comments, cg.List...)
			}

			blocks := parseCommentBlocks(comments)
			fmt.Printf("Found %d blocks in file %s\n", len(blocks), file.Name)

			localNs := groupNamespaces(blocks)

			for _, ns := range localNs {
				for i := range ns.Entries {
					parseEntry(&ns.Entries[i])
				}
				for _, cls := range ns.Classes {
					for i := range cls.Methods {
						parseEntry(&cls.Methods[i])
					}
				}
			}

			namespaces = append(namespaces, localNs...)

			// DEBUG
			debug(localNs)
		}
	}
	return namespaces, nil
}

func WriteDTS(ns []*Namespace) string {
	var out strings.Builder

	for _, n := range ns {
		n.Render(&out, "")
	}

	return out.String()
}

func debug(ns []*Namespace) {
	for _, ns := range ns {
		fmt.Printf("Namespace: %s\n", ns.Name)

		// Top-level functions, interfaces, etc.
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

		// Classes and methods
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
		// fmt.Printf("comment line: %q\n", line)
	}

	if current != nil {
		tags = append(tags, *current)
	}

	return tags
}

type Identifier struct {
	Namespace string
	Class     string
	Member    string
}

type Namespace struct {
	Name     string
	DocLines []string
	Entries  []TsEntry
	Classes  []*TsClass
}
type ParamType struct {
	Name string
	Type string
	Desc string
}
type ReturnType struct {
	Type string
	Desc string
}

type TsEntry struct {
	Kind    string
	Header  string   // the header e.g. @ts function tpl
	Content []string // raw content

	DocLines  []string
	Params    []ParamType
	Returns   *ReturnType
	Example   string
	Signature string
}

type TsClass struct {
	Name     string
	DocLines []string
	Methods  []TsEntry
}

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
				Name:     id.Namespace,
				DocLines: []string{},
				Entries:  []TsEntry{},
				Classes:  []*TsClass{},
			}
			nsMap[id.Namespace] = ns
			namespaces = append(namespaces, ns)
		}

		switch tagType {
		case "namespace":
			ns.DocLines = block.Content

		case "class":
			class := &TsClass{
				Name:     id.Class,
				DocLines: block.Content,
				Methods:  []TsEntry{},
			}
			ns.Classes = append(ns.Classes, class)
		case "method":
			for _, cls := range ns.Classes {
				if cls.Name == id.Class {
					cls.Methods = append(cls.Methods, TsEntry{
						Kind:    "method",
						Header:  block.TagLine,
						Content: block.Content,
					})
					break
				}
			}
		default: // function, interface, ...
			ns.Entries = append(ns.Entries, TsEntry{
				Kind:    tagType,
				Header:  block.TagLine,
				Content: block.Content,
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
				if param != nil {
					entry.Params = append(entry.Params, *param)
				}
			}
		case "@returns":
			entry.Returns = parseReturnLine(strings.Join(buffer, " "))
		case "@example":
			entry.Example = strings.Join(buffer, "\n")
		case "@sig":
			entry.Signature = strings.Join(buffer, " ")
		}
		buffer = nil
	}

	for _, line := range entry.Content {
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
		return fmt.Sprintf("constructor (%s): %s", strings.Join(params, ", "), ret.Type)
	case "function", "method":
		return fmt.Sprintf("export function %s (%s): %s", name, strings.Join(params, ", "), ret.Type)
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

func parseParamLine(line string) *ParamType {
	parts := strings.SplitN(line, "-", 3)

	if len(parts) < 2 {
		fmt.Printf("Warning: param line malformed: %q\n", line)
		return nil
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

	return &ParamType{
		Name: name,
		Type: typ,
		Desc: desc,
	}
}

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

func writeDoc(out *strings.Builder, indent string, docLines []string, params []ParamType, returns *ReturnType) {
	if len(docLines) == 0 && len(params) == 0 && returns == nil {
		return
	}

	out.WriteString(indent + "/**\n")

	for _, line := range docLines {
		fmt.Fprintf(out, "%s * %s\n", indent, strings.TrimSpace(line))
	}

	for _, p := range params {
		fmt.Fprintf(out, "%s * @param %s %s\n", indent, p.Name, p.Desc)
	}

	if returns != nil && returns.Desc != "" {
		fmt.Fprintf(out, "%s * @returns %s\n", indent, returns.Desc)
	}

	out.WriteString(indent + " */\n")
}

func (ns *Namespace) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, ns.DocLines, nil, nil)

	fmt.Fprintf(out, "%sdeclare module \"%s\"{\n", indent, ns.Name)

	for _, entry := range ns.Entries {
		entry.Render(out, indent+"  ")
	}

	for _, cls := range ns.Classes {
		cls.Render(out, indent+"  ")
	}

	fmt.Fprintf(out, "%s}\n", indent)

}

func (e *TsEntry) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, e.DocLines, e.Params, e.Returns)

	if e.Signature != "" {
		out.WriteString(indent + e.Signature + "\n")
	}
}

func (cls *TsClass) Render(out *strings.Builder, indent string) {
	writeDoc(out, indent, cls.DocLines, nil, nil)

	fmt.Fprintf(out, "%sexport class %s{\n", indent, cls.Name)

	for _, method := range cls.Methods {
		method.Render(out, indent+"  ")
	}

	fmt.Fprintf(out, "%s}\n", indent)
}
