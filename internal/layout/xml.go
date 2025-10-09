package layout

import (
	"craeft/internal/log"
	"craeft/internal/pipeline"
	"encoding/xml"
	"io"
	"os"
)

type Layout struct {
	path         string
	pipelines    *pipeline.Registry
	root         *os.Root
	useHotReload bool

	// Children
	fragments []Fragment
	contents  []Content
	assets    []Asset

	// XML
	XMLName xml.Name `xml:"Layout"`
	tpl     string
	tplPath string
}

type Fragment struct {
	pipeline pipeline.PipelineScript
	root     *os.Root

	// XML
	XMLName xml.Name `xml:"Fragment"`
	Name    string   `xml:"name,attr"`
	Path    string   `xml:"src,attr"`
	Pipe    string   `xml:"pipe,attr"`
}

type Content struct {
	// XML
	XMLName   xml.Name `xml:"Content"`
	Name      string
	Path      string
	Pipe      string
	assetElms []Asset
}

type Asset struct {
	pipeline pipeline.PipelineScript

	// XML
	XMLName xml.Name `xml:"Asset"`
	Name    string   `xml:"name,attr"`
	Source  string   `xml:"src,attr"`
	Pipe    string   `xml:"pipe,attr"`
}

func (l *Layout) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	l.XMLName = start.Name
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "tpl":
			l.tpl = attr.Value
		case "path":
			l.tplPath = attr.Value
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
				a.pipeline, err = l.pipelines.Get(a.Pipe)
				if err != nil {
					log.FFatal(log.Layout).
						Str("where", "layout.asset").
						Str("type", a.Pipe).
						Str("name", a.Name).
						Err(err).Msg("pipeline not found for asset")

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
				p, err := l.pipelines.Get(f.Pipe)
				if err != nil {
					log.FFatal(log.Layout).
						Str("where", "layout.fragment").
						Str("pipe", f.Pipe).
						Str("name", f.Name).
						Err(err).Msg("pipeline not found for fragment")
				}

				f.pipeline = p
				f.root = l.root
				l.fragments = append(l.fragments, f)
			case "Content":
				var c Content
				if err := d.DecodeElement(&c, &se); err != nil {
					return err
				}
				if err := c.Validate(); err != nil {
					return err
				}
				// initialize assets pipelines
				for i, a := range c.assetElms {
					p, err := l.pipelines.Get(a.Pipe)
					if err != nil {
						log.FFatal(log.Layout).
							Str("where", "layout.content.asset").
							Str("type", a.Pipe).
							Str("name", a.Name).
							Err(err).Msg("pipeline not found for content asset")
					}
					c.assetElms[i].pipeline = p
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
		case "path":
			c.Path = attr.Value
		case "pipe":
			c.Pipe = attr.Value
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
			if t.Name.Local == "Asset" {
				var a Asset
				if err := d.DecodeElement(&a, &t); err != nil {
					return err
				}
				if err := a.Validate(); err != nil {
					return err
				}
				// pipelines are set on layout.UnmarshalXML
				c.assetElms = append(c.assetElms, a)
			}
		}
	}
	return nil
}
