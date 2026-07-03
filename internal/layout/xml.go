package layout

import (
	"encoding/xml"
	"errors"
	"io"

	"github.com/valyala/fasttemplate"
)

type Layout struct {
	path         string
	useHotReload bool

	// Children
	fragments []Fragment
	contents  []Content
	assets    []Asset

	// XML
	XMLName xml.Name `xml:"Layout"`
	tpl     string
	TplPath string

	outputs map[string]*fasttemplate.Template
}

type Fragment struct {
	// XML
	XMLName xml.Name `xml:"Fragment"`
	Name    string   `xml:"name,attr"`
	Source  string   `xml:"src,attr"`
	Pipe    string   `xml:"pipe,attr"`
	base    string
}

type Content struct {
	// XML
	XMLName   xml.Name `xml:"Content"`
	Name      string
	Source    string
	Pipe      string
	To        string
	assetElms []Asset
	vars      map[string]string
}

type Asset struct {
	// XML
	XMLName xml.Name `xml:"Asset"`
	Name    string   `xml:"name,attr"`
	Source  string   `xml:"src,attr"`
	Pipe    string   `xml:"pipe,attr"`
	To      string   `xml:"to,attr"`
}

type Var struct {
	XMLName xml.Name `xml:"Var"`
	Name    string   `xml:"name,attr"`
	Value   string   `xml:"value,attr"`
}

func (l *Layout) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	l.XMLName = start.Name
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "tpl":
			l.tpl = attr.Value
		case "src":
			l.TplPath = attr.Value
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
			case "Asset":
				var a Asset
				if err := d.DecodeElement(&a, &se); err != nil {
					return err
				}
				if err := a.Validate(); err != nil {
					return err
				}
				l.assets = append(l.assets, a)
			case "Fragment":
				var f Fragment
				if err := d.DecodeElement(&f, &se); err != nil {
					return err
				}
				if err := f.Validate(); err != nil {
					return err
				}

				f.base = l.path
				l.fragments = append(l.fragments, f)
			case "Content":
				var c Content
				if err := d.DecodeElement(&c, &se); err != nil {
					return err
				}
				if err := c.Validate(); err != nil {
					return err
				}
				l.contents = append(l.contents, c)
			}
		}
	}
	return nil
}

func (c *Content) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	c.XMLName = start.Name
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "name":
			c.Name = attr.Value
		case "src":
			c.Source = attr.Value
		case "pipe":
			c.Pipe = attr.Value
		case "to":
			c.To = attr.Value
		}
	}

	for {
		tok, err := d.Token()

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name == start.Name {
				return nil
			}
		case xml.StartElement:
			switch t.Name.Local {
			case "Asset":
				var a Asset
				if err := d.DecodeElement(&a, &t); err != nil {
					return err
				}
				if err := a.Validate(); err != nil {
					return err
				}
				// pipelines are set on layout.UnmarshalXML
				c.assetElms = append(c.assetElms, a)
			case "Var":
				var v Var
				if err := d.DecodeElement(&v, &t); err != nil {
					return err
				}
				if v.Name == "" {
					return errors.New("element <Var>: missing required field: name")
				}
				if c.vars == nil {
					c.vars = make(map[string]string)
				}
				c.vars[v.Name] = v.Value
			}
		}
	}
	return nil
}
