package project

import (
	"encoding/xml"
	"io"
	"sort"
	"strconv"
	"strings"
)

type graphmlDoc struct {
	XMLName xml.Name     `xml:"graphml"`
	XMLNS   string       `xml:"xmlns,attr"`
	XSI     string       `xml:"xmlns:xsi,attr"`
	Schema  string       `xml:"xsi:schemaLocation,attr"`
	YNS     string       `xml:"xmlns:y,attr"`
	Keys    []graphmlKey `xml:"key"`
	Graph   graphmlGraph `xml:"graph"`
}

type graphmlKey struct {
	ID       string `xml:"id,attr"`
	For      string `xml:"for,attr"`
	AttrName string `xml:"attr.name,attr"`
	AttrType string `xml:"attr.type,attr"`
}

type graphmlGraph struct {
	ID          string        `xml:"id,attr"`
	EdgeDefault string        `xml:"edgedefault,attr"`
	Data        []graphmlData `xml:"data"`
	Nodes       []graphmlNode `xml:"node"`
	Edges       []graphmlEdge `xml:"edge"`
}

type graphmlNode struct {
	ID    string        `xml:"id,attr"`
	Data  []graphmlData `xml:"data"`
}

type graphmlEdge struct {
	ID     string `xml:"id,attr"`
	Source string `xml:"source,attr"`
	Target string `xml:"target,attr"`
}

type graphmlData struct {
	Key   string `xml:"key,attr"`
	Value string `xml:",chardata"`
}

// nodeAttrs holds the display properties we derive from each Node.
type nodeAttrs struct {
	label     string
	kind      string
	color     string
	entryType string // "content" | "asset" | ""
	baseDir   string // only set for entry nodes
}

// WriteGraphML renders the dependency graph in GraphML format and writes it to w
func WriteGraphML(w io.Writer, g DependencyGraph) error {
	nodes := g.AllNodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID() < nodes[j].ID() })

	doc := graphmlDoc{
		XMLNS:  "http://graphml.graphdrawing.org/graphml",
		XSI:    "http://www.w3.org/2001/XMLSchema-instance",
		Schema: "http://graphml.graphdrawing.org/graphml http://graphml.graphdrawing.org/graphml/graphml.xsd",
		YNS:    "http://www.yworks.com/xml/graphml",
		Keys: []graphmlKey{
			{ID: "d_label", For: "node", AttrName: "label", AttrType: "string"},
			{ID: "d_kind", For: "node", AttrName: "kind", AttrType: "string"},
			{ID: "d_color", For: "node", AttrName: "color", AttrType: "string"},
			{ID: "d_entry_type", For: "node", AttrName: "entry_type", AttrType: "string"},
			{ID: "d_base_dir", For: "node", AttrName: "base_dir", AttrType: "string"},
			{ID: "d_title", For: "graph", AttrName: "title", AttrType: "string"},
		},
	}

	var xmlNodes []graphmlNode
	var xmlEdges []graphmlEdge

	for _, n := range nodes {
		xmlNodes = append(xmlNodes, toXMLNode(n))
	}

	edgeIdx := 0
	for _, n := range nodes {
		deps := g.DependenciesOf(n)
		sort.Slice(deps, func(i, j int) bool { return deps[i].ID() < deps[j].ID() })
		for _, dep := range deps {
			xmlEdges = append(xmlEdges, graphmlEdge{
				ID:     edgeID(edgeIdx),
				Source: xmlID(n.ID()),
				Target: xmlID(dep.ID()),
			})
			edgeIdx++
		}
	}

	doc.Graph = graphmlGraph{
		ID:          "G",
		EdgeDefault: "directed",
		Nodes:       xmlNodes,
		Edges:       xmlEdges,
	}

	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	return enc.Encode(doc)
}

// toXMLNode converts a Node to its graphmlNode representation.
func toXMLNode(n Node) graphmlNode {
	a := nodeAttributes(n)
	data := []graphmlData{
		{Key: "d_label", Value: a.label},
		{Key: "d_kind", Value: a.kind},
		{Key: "d_color", Value: a.color},
	}
	if a.entryType != "" {
		data = append(data, graphmlData{Key: "d_entry_type", Value: a.entryType})
	}
	if a.baseDir != "" {
		data = append(data, graphmlData{Key: "d_base_dir", Value: a.baseDir})
	}
	return graphmlNode{
		ID:   xmlID(n.ID()),
		Data: data,
	}
}

// nodeAttributes derives display properties from any Node.
func nodeAttributes(n Node) nodeAttrs {
	switch n.Kind() {
	case NodeKindEntry:
		en := n.(*EntryNode)
		e := en.Entry()
		label := shortLabel(e.SourcePath)
		if label == "" {
			label = e.ID
		}
		color := "#93c5fd" // blue-100 – content
		entryType := "content"
		if e.Type == AssetType {
			color = "#fdba74" // orange-100 – asset
			entryType = "asset"
		}
		return nodeAttrs{
			label:     label,
			kind:      string(NodeKindEntry),
			color:     color,
			entryType: entryType,
			baseDir:   e.BaseDir,
		}

	case NodeKindLayout:
		ln := n.(*LayoutNode)
		label := shortLabel(ln.Key().LayoutPath)
		if ctx := ln.Key().CtxPath; ctx != "" && ctx != "." {
			label += " (" + ctx + ")"
		}
		return nodeAttrs{label: label, kind: string(NodeKindLayout), color: "#86efac"}

	case NodeKindFragment:
		fn := n.(*FragmentNode)
		return nodeAttrs{
			label: shortLabel(fn.Path()),
			kind:  string(NodeKindFragment),
			color: "#d8b4fe",
		}

	case NodeKindTemplate:
		tn := n.(*TemplateNode)
		return nodeAttrs{
			label: shortLabel(tn.Path()),
			kind:  string(NodeKindTemplate),
			color: "#5eead4",
		}

	case NodeKindPipeline:
		pn := n.(*PipelineNode)
		return nodeAttrs{label: pn.Name(), kind: string(NodeKindPipeline), color: "#fca5a5"}

	default:
		return nodeAttrs{label: n.ID(), kind: "unknown", color: "#d1d5db"}
	}
}

// shortLabel returns the last path component, keeping labels readable.
func shortLabel(path string) string {
	path = strings.ReplaceAll(path, `\`, `/`)
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

// xmlID returns a valid XML id attribute value for any node ID string.
// GraphML node ids must be valid XML NCNames in strict parsers, so we
// prefix with "n_" and replace every non-alphanumeric character with "_".
func xmlID(s string) string {
	var b strings.Builder
	b.WriteString("n_")
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// xmlSafeID is like xmlID but without the "n_" prefix, for group IDs.
func xmlSafeID(s string) string {
	return strings.TrimPrefix(xmlID(s), "n_")
}

func edgeID(i int) string {
	return "e_" + strconv.Itoa(i)
}
