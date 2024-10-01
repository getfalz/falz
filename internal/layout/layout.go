package layout

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"craeft/internal/utils"

	log "github.com/sirupsen/logrus"
	"github.com/valyala/fasttemplate"
)

type ctxKey string

const ctxBasePath = ctxKey("basePath")

type Layout struct {
	XMLName        xml.Name `xml:"Layout"`
	basePath       string
	path           string
	contentPattern string
	fragments      []string
	output         string
	Child          Renderable `xml:",any"`
	CSS            string     `xml:"css,attr"`
	JS             string     `xml:"js,attr"`
	Tpl            string     `xml:"template,attr"`
}

type Renderable interface {
	Render(context.Context) (string, error)
}

type Item struct {
	XMLName  xml.Name     `xml:"Item"`
	Width    string       `xml:"width,attr"`
	Height   string       `xml:"height,attr"`
	HExpand  bool         `xml:"hexpand,attr"`
	VExpand  bool         `xml:"vexpand,attr"`
	Span     int          `xml:"span,attr"`
	Children []Renderable `xml:",any"`
}

type Flow struct {
	XMLName   xml.Name `xml:"Flow"`
	Direction string   `xml:"direction,attr"`
	Align     string   `xml:"align,attr"`
	Wrap      bool     `xml:"wrap,attr"`
	Items     []Item   `xml:"Item"`
}

type Grid struct {
	XMLName xml.Name `xml:"Grid"`
	Columns int      `xml:"columns,attr"`
	Gap     string   `xml:"gap,attr"`
	Items   []Item   `xml:"Item"`
}

type Fragment struct {
	XMLName  xml.Name `xml:"Fragment"`
	Source   string   `xml:"src,attr"`
	Pipeline string   `xml:"pipeline,attr"`
}

type Content struct {
	XMLName   xml.Name   `xml:"Content"`
	Path      string     `xml:"path,attr"`
	Pipeline  string     `xml:"pipeline,attr"`
	Variables []Variable `xml:"Variable"`
}

type Variable struct {
	XMLName xml.Name `xml:"Variable"`
	Name    string   `xml:"name,attr"`
}

func parseTpl(tpl string, data map[string]any) string {
	// TODO use config for delimiters
	t := fasttemplate.New(tpl, "{{", "}}")
	return t.ExecuteString(data)
}

// Item struct renders its children (if any) or standalone
func (i Item) Render(ctx context.Context) (string, error) {
	// Render child elements
	var childrenHTML strings.Builder
	for _, child := range i.Children {
		childHTML, err := child.Render(ctx)
		if err != nil {
			return "", err
		}
		childrenHTML.WriteString(childHTML)
	}

	// Use a Go template to generate the HTML for the Item
	tmpl := `<div class="item" style="width: {{Width}}; height: {{Height}};">
                {{Children}}
             </div>`

	result := parseTpl(tmpl, map[string]any{
		"Width":    i.Width,
		"Height":   i.Height,
		"Children": childrenHTML.String(),
	})
	// var rendered bytes.Buffer
	// t := template.Must(template.New("item").Parse(tmpl))
	// err := t.Execute(&rendered, map[string]interface{}{
	// 	"Width":    i.Width,
	// 	"Height":   i.Height,
	// 	"Children": template.HTML(childrenHTML.String()),
	// })
	// if err != nil {
	// 	return "", err
	// }
	// return rendered.String(), nil
	return result, nil
}

// Flow struct renders its child items in a flexbox layout
func (f Flow) Render(ctx context.Context) (string, error) {
	var itemsHTML strings.Builder
	for _, item := range f.Items {
		itemHTML, err := item.Render(ctx)
		if err != nil {
			return "", err
		}
		itemsHTML.WriteString(itemHTML)
	}

	// Use a Go template for the Flow layout
	tmpl := `<div class="flow" style="flex-direction: {{Direction}}; align-items: {{Align}};">
                {{Items}}
             </div>`

	result := parseTpl(tmpl, map[string]any{
		"Direction": f.Direction,
		"Align":     f.Align,
		"Items":     itemsHTML.String(),
	})

	// var rendered bytes.Buffer
	// t := template.Must(template.New("flow").Parse(tmpl))
	// err := t.Execute(&rendered, map[string]interface{}{
	// 	"Direction": f.Direction,
	// 	"Align":     f.Align,
	// 	"Items":     template.HTML(itemsHTML.String()),
	// })
	// if err != nil {
	// 	return "", err
	// }
	// return rendered.String(), nil
	return result, nil
}

// Grid struct renders its child items in a grid layout
func (g Grid) Render(ctx context.Context) (string, error) {
	var itemsHTML strings.Builder
	for _, item := range g.Items {
		itemHTML, err := item.Render(ctx)
		if err != nil {
			return "", err
		}
		itemsHTML.WriteString(itemHTML)
	}

	// Use a Go template for the Grid layout
	tmpl := `<div class="grid" style="grid-template-columns: repeat({{Columns}}, 1fr); gap: {{Gap}};">
                {{Items}}
             </div>`

	result := parseTpl(tmpl, map[string]any{
		"Columns": g.Columns,
		"Gap":     g.Gap,
		"Items":   itemsHTML.String(),
	})

	// var rendered bytes.Buffer
	// t := template.Must(template.New("grid").Parse(tmpl))
	// err := t.Execute(&rendered, map[string]interface{}{
	// 	"Columns": g.Columns,
	// 	"Gap":     g.Gap,
	// 	"Items":   template.HTML(itemsHTML.String()),
	// })
	// if err != nil {
	// 	return "", err
	// }
	// return rendered.String(), nil
	return result, nil
}

// Fragment struct loads and renders raw HTML from a file
func (f Fragment) Render(ctx context.Context) (string, error) {
	basePathValue := ctx.Value(ctxBasePath)
	basePath, ok := basePathValue.(string)
	if !ok {
		return "", fmt.Errorf("BasePath not found in context or is not a string")
	}

	srcPath, err := utils.FindFile(basePath, "", f.Source)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}
	return string(content), nil // Directly return the raw HTML content
}

// Content struct loads Markdown, converts it to HTML, and renders it
func (c Content) Render(ctx context.Context) (string, error) {
	// TODO use config delimiters
	return "{{.Content}}", nil
}

// Render layout. The fragments will be searched first in the given base path
func (l *Layout) Render(base string) (string, error) {
	// TODO render websocket script only when the development flag is set
	// but find a way to define the variables placeholders. maybe use fasttemplate?

	if l.output != "" {
		return l.output, nil
	}
	tplPath, err := utils.FindFile(l.basePath, base, l.Tpl)
	if err != nil {
		return "", err
	}

	tmpl, err := os.ReadFile(tplPath)
	if err != nil {
		return "", err
	}

	ctx := context.WithValue(context.Background(), ctxBasePath, l.basePath)
	body, err := l.Child.Render(ctx)
	if err != nil {
		log.Error(err)
		body = ""
	}

	result := parseTpl(string(tmpl), map[string]any{
		"CSS":  l.CSS,
		"JS":   l.JS,
		"Body": body,
	})

	// var rendered bytes.Buffer
	// t := template.Must(template.New("main").Parse(string(tmpl)))
	// // TODO check that css/js are only file names and no path to other folder
	// err = t.Execute(&rendered, map[string]interface{}{
	// 	"CSS":  l.CSS,
	// 	"JS":   l.JS,
	// 	"Body": template.HTML(body),
	// })
	// if err != nil {
	// 	return "", err
	// }
	// l.output = rendered.String()

	l.output = result
	return l.output, nil
}

func (l *Layout) Fragments() []string {
	if len(l.fragments) != 0 {
		return l.fragments
	}
	l.fragments = l.collectFragments(l.Child)

	return l.fragments
}

func (l Layout) collectFragments(child Renderable) []string {
	var fragments []string
	switch v := child.(type) {
	case *Fragment:
		fragments = append(fragments, v.Source)
	case *Flow:
		for _, item := range v.Items {
			item.collectFragments(&fragments)
		}
	case *Grid:
		for _, item := range v.Items {
			item.collectFragments(&fragments)
		}
	}

	return fragments
}

// TODO make test to check if nested children can have contents
func (i Item) collectFragments(fragments *[]string) {
	if i.Children != nil {
		for _, child := range i.Children {
			switch v := child.(type) {
			case *Fragment:
				*fragments = append(*fragments, v.Source)
			}
		}
	}
}

func (l *Layout) ContentPattern() string {
	if l.contentPattern != "" {
		return l.contentPattern
	}
	l.contentPattern = l.collectContents(l.Child)

	return l.contentPattern
}

func (l Layout) collectContents(child Renderable) string {
	var contentPattern string
	switch v := child.(type) {
	case *Content:
		contentPattern = v.Path
	case *Flow:
		for _, item := range v.Items {
			if r := item.collectContents(); r != "" {
				contentPattern = r
				break
			}
		}
	case *Grid:
		for _, item := range v.Items {
			contentPattern = item.collectContents()
		}
	}

	return contentPattern
}

func (i Item) collectContents() string {
	var contentPattern string
	if i.Children != nil {
		for _, child := range i.Children {
			switch v := child.(type) {
			case *Flow:
				for _, item := range v.Items {
					if r := item.collectContents(); r != "" {
						contentPattern = r
						break
					}
				}
			case *Grid:
				for _, item := range v.Items {
					if r := item.collectContents(); r != "" {
						contentPattern = r
						break
					}
				}
			case *Content:
				contentPattern = v.Path
			}
		}
	}
	return contentPattern
}

func (l *Layout) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	l.XMLName = start.Name
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "js":
			l.JS = attr.Value
		case "css":
			l.CSS = attr.Value
		case "template":
			l.Tpl = attr.Value
		}
	}

	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch se := t.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "Flow":
				var child Flow
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				l.Child = &child
			case "Grid":
				var child Grid
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				l.Child = &child
			case "Fragment":
				var child Fragment
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				l.Child = &child
			case "Content":
				var child Content
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				l.Child = &child
			}
		}
	}
	return nil
}

func (i *Item) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	i.XMLName = start.Name
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "width":
			i.Width = attr.Value
		case "height":
			i.Height = attr.Value
		case "hexpand":
			i.HExpand = attr.Value == "true"
		case "vexpand":
			i.VExpand = attr.Value == "true"
		case "span":
			fmt.Sscanf(attr.Value, "%d", &i.Span)
		}
	}
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch se := t.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "Flow":
				var child Flow
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				i.Children = append(i.Children, &child)
			case "Grid":
				var child Grid
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				i.Children = append(i.Children, &child)
			case "Fragment":
				var child Fragment
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				i.Children = append(i.Children, &child)
			case "Content":
				var child Content
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				i.Children = append(i.Children, &child)
			}
		}
	}
	return nil
}

func (c *Content) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	// custom xml parser needed to ignore the closing tag

	c.XMLName = start.Name
	// Loop through attributes to extract values
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "path":
			c.Path = attr.Value
		case "pipeline":
			c.Pipeline = attr.Value
		}
	}

	// Check for self-closing tag
	for {
		tok, _ := d.Token()
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name == start.Name {
				// Explicitly closed empty element
				return nil
			}
		case xml.StartElement:
			// Handle other nested elements, if needed (e.g., Variables)
			if t.Name.Local == "Variable" {
				var v Variable
				if err := d.DecodeElement(&v, &t); err != nil {
					return err
				}
				c.Variables = append(c.Variables, v)
			}
		}
	}
}

func (l Layout) Path() string {
	return l.path
}

func (l Layout) HasPart(part string) bool {
	return false
}

// Parse parses a layout.xml file. The basePath is the root of the project, and cwd is the path of the layout.xml and should be relative to project path
func Parse(basePath string, cwd string, data []byte) (*Layout, error) {
	layout := &Layout{basePath: basePath, path: cwd}
	err := xml.Unmarshal(data, layout)
	if err != nil {
		return nil, err
	}
	return layout, nil
}
