package layout

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/mattn/go-zglob"
	log "github.com/sirupsen/logrus"
	"github.com/valyala/fasttemplate"
)

type ctxKey string

type contextKey int

const (
	ctxBasePath contextKey = iota
	ctxRenderPath
	ctxLayoutPath
	ctxOutputPath
)

// TODO make xml structs private?
type Layout struct {
	XMLName   xml.Name `xml:"Layout"`
	basePath  string
	path      string
	pipelines *pipeline.Registry

	// Children
	fragments []Fragment
	content   *Content
	assets    []Asset

	Tpl string `xml:"template,attr"`
}

type Fragment struct {
	pipeline pipeline.PipelineScript

	XMLName  xml.Name `xml:"Fragment"`
	Name     string   `xml:"name,attr"`
	Source   string   `xml:"src,attr"`
	Pipeline string   `xml:"pipeline,attr"`
}

type Content struct {
	XMLName   xml.Name   `xml:"Content"`
	Name      string     `xml:"name,attr"`
	Path      string     `xml:"path,attr"`
	Pipeline  string     `xml:"pipeline,attr"`
	Variables []Variable `xml:"Variable"`
}

type Variable struct {
	XMLName xml.Name `xml:"Variable"`
	Name    string   `xml:"name,attr"`
}

type Asset struct {
	pipeline pipeline.PipelineScript

	XMLName   xml.Name `xml:"Asset"`
	Name      string   `xml:"name,attr"`
	Source    string   `xml:"src,attr"`
	AssetType string   `xml:"type,attr"`
}

func parseTpl(tpl string, data map[string]any) string {
	// TODO use config for delimiters
	t := fasttemplate.New(tpl, "{{", "}}")
	return t.ExecuteString(data)
}

// Fragment struct loads and renders raw HTML from a file
func (f Fragment) Render(ctx context.Context) (string, error) {
	basePath := getValue(ctx, ctxBasePath)
	renderPath := getValue(ctx, ctxRenderPath)

	files, err := utils.FindFile(basePath, renderPath, f.Source)
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("No files found for fragment %s", f.Source)
	}

	var content []byte
	for _, path := range files {
		content, err = os.ReadFile(path)
		if err != nil {
			return "", err
		}
		break
	}

	// TODO should we pass any vars to the pipeline?
	result := f.pipeline.Transform(string(content), nil)

	if len(result) == 0 {
		return "", fmt.Errorf("Fragments pipeline %s returned no result", f.pipeline.Config().PipelineName)
	}
	if len(result) > 1 {
		log.Warnf("Fragments pipeline %s returned more than one result. Using the first one", f.pipeline.Config().PipelineName)
	}

	return result[0]["result"], nil
}

func getValue(ctx context.Context, key contextKey) string {
	// Use the key to get the value from the context
	if val, ok := ctx.Value(key).(string); ok {
		return val
	}
	return ""
}

// Content struct loads Markdown, converts it to HTML, and renders it
func (c Content) Render(ctx context.Context) (string, error) {
	// TODO use config delimiters
	return "[[layout]]", nil
}

func (a Asset) Render(ctx context.Context) (string, error) {
	basePath := getValue(ctx, ctxBasePath)

	// TODO a.Source can be a pattern (eg. *.css) so we need to handle that
	target := filepath.Join(basePath, a.Source)

	var tmpl string
	switch a.AssetType {
	case "css":
		tmpl = `<link rel="stylesheet" href="{{Target}}" />`
	case "js":
		tmpl = `<script type="application/javascript" src="{{Target}}"></script>`
	default:
		tmpl = `{{Target}}`
	}

	result := parseTpl(tmpl, map[string]any{
		"Target": target,
	})

	return string(result), nil
}

// Render layout. The fragments will be searched first in the given base path
func (l *Layout) Render(path string) (string, error) {
	// TODO render websocket script only when the development flag is set
	// but find a way to define the variables placeholders. maybe use fasttemplate?

	tplPath, err := utils.FindFile(l.basePath, path, l.Tpl)
	if err != nil {
		return "", fmt.Errorf("Can't find layout template: %w", err)
	}

	var tmpl []byte
	for _, tplPath := range tplPath {
		tmpl, err = os.ReadFile(tplPath)
		if err != nil {
			return "", err
		}
		break
	}

	ctx := context.WithValue(context.Background(), ctxBasePath, l.basePath)
	ctx = context.WithValue(ctx, ctxRenderPath, path)
	ctx = context.WithValue(ctx, ctxLayoutPath, l.path)

	tplVariables := make(map[string]any)
	for _, asset := range l.assets {
		output, err := asset.Render(ctx)
		if err != nil {
			return "", err
		}
		tplVariables[asset.Name] = output
	}

	content, err := l.content.Render(ctx)
	if err != nil {
		return "", err
	}
	tplVariables[l.content.Name] = content

	for _, f := range l.fragments {
		output, err := f.Render(ctx)
		if err != nil {
			return "", err
		}
		tplVariables[f.Name] = output
	}

	log.Infof("Rendering layout %s with variables: %v", l.Tpl, tplVariables)
	result := parseTpl(string(tmpl), tplVariables)

	return result, nil
}

func (l *Layout) Fragments() []string {
	var fragments []string

	for _, f := range l.fragments {
		fragments = append(fragments, f.Source)
	}

	return fragments
}

func (l Layout) Content() Content {
	return *l.content
}

func (l Layout) Assets() []Asset {
	return l.assets
}

func (l *Layout) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	l.XMLName = start.Name
	for _, attr := range start.Attr {
		switch attr.Name.Local {
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
			case "Asset":
				var v Asset
				if err := d.DecodeElement(&v, &se); err != nil {
					return err
				}
				v.pipeline, err = l.pipelines.Get(v.AssetType, true)
				if err != nil {
					log.Warnf("pipeline '%s' not found for asset %s: %v. Using default", v.AssetType, v.Name, err)
				}
				l.assets = append(l.assets, v)
			case "Fragment":
				var v Fragment
				if err := d.DecodeElement(&v, &se); err != nil {
					return err
				}
				v.pipeline, err = l.pipelines.Get(v.Pipeline, true)
				if err != nil {
					log.Warnf("pipeline '%s' not found for Fragment %s: %v. Using default", v.Pipeline, v.Name, err)
				}
				l.fragments = append(l.fragments, v)
			case "Content":
				var child Content
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				l.content = &child
			}
		}
	}
	return nil
}

// Path is the relative path of the layout to the project root folder.
func (l Layout) Path() string {
	return l.path
}

func (l Layout) HasPart(part string) bool {
	// TODO like collectContents, collectFragments
	return false
}

// MatchContent checks if the given path matches the layout's content
func (l Layout) MatchContent(path string) bool {
	match, err := zglob.Match(l.content.Path, path)
	if err != nil {
		log.Errorf("Error matching layout with path %s: %v\n", path, err)
		return false
	}

	return match
}

// Parse parses a layout.xml file. The basePath is the root of the project, and cwd is the path of the layout.xml and should be relative to project path
func Parse(basePath string, cwd string, data []byte, pipeReg *pipeline.Registry) (*Layout, error) {
	layout := &Layout{basePath: basePath, path: cwd, pipelines: pipeReg}
	err := xml.Unmarshal(data, layout)
	if err != nil {
		return nil, err
	}

	return layout, nil
}
