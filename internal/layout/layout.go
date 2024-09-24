package layout

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

type Layout struct {
	XMLName xml.Name   `xml:"Layout"`
	Child   Renderable `xml:",any"` // Child can be Fragment, Content, Grid, or Flow
	CSS     string     `xml:"css,attr"`
	JS      string     `xml:"js,attr"`
}

type Renderable interface {
	Render() (string, error)
}

type Item struct {
	XMLName  xml.Name     `xml:"Item"`
	Width    string       `xml:"width,attr"`
	Height   string       `xml:"height,attr"`
	HExpand  bool         `xml:"hexpand,attr"`
	VExpand  bool         `xml:"vexpand,attr"`
	Span     int          `xml:"span,attr"`
	Children []Renderable `xml:",any"` // Can hold Flow, Grid, Fragment, or Content
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

// Fragment struct represents an HTML fragment
type Fragment struct {
	XMLName xml.Name `xml:"Fragment"`
	Source  string   `xml:"source,attr"` // Path to the fragment file
}

// Content struct represents Markdown content
type Content struct {
	XMLName xml.Name `xml:"Content"`
	Source  string   `xml:"source,attr"` // Path to the markdown file
}

// Item struct renders its children (if any) or standalone
func (i Item) Render() (string, error) {
	// Render child elements
	var childrenHTML strings.Builder
	for _, child := range i.Children {
		childHTML, err := child.Render()
		if err != nil {
			return "", err
		}
		childrenHTML.WriteString(childHTML)
	}

	// Use a Go template to generate the HTML for the Item
	tmpl := `<div class="item" style="width: {{.Width}}; height: {{.Height}};">
                {{.Children}}
             </div>`

	var rendered bytes.Buffer
	t := template.Must(template.New("item").Parse(tmpl))
	err := t.Execute(&rendered, map[string]interface{}{
		"Width":    i.Width,
		"Height":   i.Height,
		"Children": template.HTML(childrenHTML.String()),
	})
	if err != nil {
		return "", err
	}
	return rendered.String(), nil
}

// Flow struct renders its child items in a flexbox layout
func (f Flow) Render() (string, error) {
	var itemsHTML strings.Builder
	for _, item := range f.Items {
		itemHTML, err := item.Render()
		if err != nil {
			return "", err
		}
		itemsHTML.WriteString(itemHTML)
	}

	// Use a Go template for the Flow layout
	tmpl := `<div class="flow" style="flex-direction: {{.Direction}}; align-items: {{.Align}};">
                {{.Items}}
             </div>`

	var rendered bytes.Buffer
	t := template.Must(template.New("flow").Parse(tmpl))
	err := t.Execute(&rendered, map[string]interface{}{
		"Direction": f.Direction,
		"Align":     f.Align,
		"Items":     template.HTML(itemsHTML.String()),
	})
	if err != nil {
		return "", err
	}
	return rendered.String(), nil
}

// Grid struct renders its child items in a grid layout
func (g Grid) Render() (string, error) {
	var itemsHTML strings.Builder
	for _, item := range g.Items {
		itemHTML, err := item.Render()
		if err != nil {
			return "", err
		}
		itemsHTML.WriteString(itemHTML)
	}

	// Use a Go template for the Grid layout
	tmpl := `<div class="grid" style="grid-template-columns: repeat({{.Columns}}, 1fr); gap: {{.Gap}};">
                {{.Items}}
             </div>`

	var rendered bytes.Buffer
	t := template.Must(template.New("grid").Parse(tmpl))
	err := t.Execute(&rendered, map[string]interface{}{
		"Columns": g.Columns,
		"Gap":     g.Gap,
		"Items":   template.HTML(itemsHTML.String()),
	})
	if err != nil {
		return "", err
	}
	return rendered.String(), nil
}

// Fragment struct loads and renders raw HTML from a file
func (f Fragment) Render() (string, error) {
	// TODO the file should be searched using tools.FindFile
	content, err := os.ReadFile(f.Source)
	if err != nil {
		return "", err
	}
	return string(content), nil // Directly return the raw HTML content
}

// Content struct loads Markdown, converts it to HTML, and renders it
func (c Content) Render() (string, error) {
	return "{{.Content}}", nil
}

// TODO cache render output
func (l Layout) Render(path string) (string, error) {
	// TODO the layout template should be declared on xml. that way, the system is independent of the output.
	// TODO render websocket script only when the development flag is set
	// but find a way to define the variables placeholders. maybe use fasttemplate?
	tmpl := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Title</title>
    <link rel="stylesheet" href="{{.CSS}}">
    <script src="{{.JS}}" defer></script>
    <script>
    (() => {
        const socketUrl = 'ws://'+window.location.host+'/ws';
        let socket = new WebSocket(socketUrl);
        socket.addEventListener('message', (evt) => {
            let data = JSON.parse(evt.data)
            window.location.href = data.url;
        });
    })();
    </script>
</head>
<body>
    {{.Body}}
</body>
</html>
`

	body, err := l.Child.Render()
	if err != nil {
		log.Error(err)
		body = ""
	}

	var rendered bytes.Buffer
	t := template.Must(template.New("main").Parse(tmpl))
	err = t.Execute(&rendered, map[string]interface{}{
		"CSS":  l.CSS,
		"JS":   l.JS,
		"Body": template.HTML(body),
	})
	if err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func (l Layout) GetContents() map[string]string {
	contents := make(map[string]string)
	l.collectContents(contents, l.Child)
	return contents
}

// Collect contents from Renderable child elements
func (l Layout) collectContents(contents map[string]string, child Renderable) {
	switch v := child.(type) {
	case *Content:
		contents[v.Source] = v.Source // Add Content source to the map

	case *Flow:
		for _, item := range v.Items {
			item.collectContents(contents)
		}

	case *Grid:
		for _, item := range v.Items {
			item.collectContents(contents)
		}
	}
}

// Add a method to Item to collect its contents
func (i Item) collectContents(contents map[string]string) {
	if i.Children != nil {
		for _, child := range i.Children {
			switch v := child.(type) {
			case *Content:
				contents[v.Source] = v.Source
			}
		}
	}
}

func (l *Layout) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	l.XMLName = start.Name
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "js":
			l.JS = attr.Value
		case "css":
			l.CSS = attr.Value
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

func Parse(data []byte) (*Layout, error) {
	layout := &Layout{}
	err := xml.Unmarshal(data, layout)
	if err != nil {
		return nil, err
	}
	return layout, nil
}
