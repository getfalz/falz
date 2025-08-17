package layout

import (
	"craeft/internal/log"
	"encoding/xml"
	"fmt"
	"io"
)

func (l *Layout) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	l.XMLName = start.Name
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "tpl":
			l.Tpl = attr.Value
		case "path":
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
				a.pipeline, err = l.pipelines.Get(a.AssetType)
				if err != nil {
					log.Warn(
						fmt.Sprintf("pipeline '%s' not found for asset %s: %s", a.AssetType, a.Name, err),
						log.Tag{Key: log.SrcTag, Value: "layout"},
					)
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
				f.pipeline, err = l.pipelines.Get(f.Pipe)
				f.root = l.root
				if err != nil {
					log.Warn(
						fmt.Sprintf("pipeline '%s' not found for fragment %s, err: %s", f.Pipe, f.Name, err),
						log.Tag{Key: log.SrcTag, Value: "layout"},
					)
				}
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
				for i, a := range c.Assets {
					c.Assets[i].pipeline, err = l.pipelines.Get(a.AssetType)
					if err != nil {
						log.Warn(
							fmt.Sprintf("pipeline '%s' not found for content asset %s, err: %s", a.AssetType, a.Name, err),
							log.Tag{Key: log.SrcTag, Value: "layout"},
						)
					}
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
				c.Assets = append(c.Assets, a)
			}
		}
	}
	return nil
}
