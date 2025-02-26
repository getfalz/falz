package layout

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/mattn/go-zglob"
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

const wsClient string = `
<script>
(() => {
    const socketUrl = 'ws://'+window.location.host+'/ws';
    let socket = new WebSocket(socketUrl);
        socket.addEventListener('message', (evt) => {
        let data = JSON.parse(evt.data)
        if (data.reload) {
            window.location.reload();
        } else {
            window.location.href = data.url;
        }
    });
})();
</script>

`

// TODO make xml structs private?
type Layout struct {
	XMLName      xml.Name `xml:"Layout"`
	basePath     string
	name         string
	path         string
	pipelines    *pipeline.Registry
	useHotReload bool

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
	XMLName  xml.Name `xml:"Content"`
	Name     string   `xml:"name,attr"`
	Path     string   `xml:"path,attr"`
	Pipeline string   `xml:"pipeline,attr"`
	Assets   []Asset  `xml:"Assets"`
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
		return "", fmt.Errorf("no files found for fragment %s", f.Source)
	}

	var content []byte
	content, err = os.ReadFile(filepath.Join(basePath, files[0]))
	if err != nil {
		return "", err
	}

	vars := pipeline.NewVars("")
	result := f.pipeline.Transform(content, vars)

	if len(result) == 0 {
		return "", fmt.Errorf("pipeline '%s' for fragment '%s' returned an empty result", f.pipeline.Config().PipelineName, f.Name)
	}
	if len(result) > 1 {
		log.Warn(fmt.Sprintf("Fragments pipeline %s returned more than one result. Using the first one", f.pipeline.Config().PipelineName), log.Tag{Key: log.PipelineTag, Value: f.Name})
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
	return fmt.Sprintf("{{%s}}", c.Name), nil
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

	tplPaths, err := utils.FindFile(l.basePath, path, l.Tpl)
	if err != nil {
		return "", fmt.Errorf("Can't find layout template: %w", err)
	}

	var tmpl []byte
	tmpl, err = os.ReadFile(filepath.Join(l.basePath, tplPaths[0]))
	if err != nil {
		return "", err
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

	if l.useHotReload {
		tplVariables["hotreload"] = wsClient
	}

	log.Trace(fmt.Sprintf("Rendering layout %s with variables: %v", l.Tpl, tplVariables))
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

func (c *Content) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	c.XMLName = start.Name
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "name":
			c.Name = attr.Value
		case "path":
			c.Path = attr.Value
		case "pipeline":
			c.Pipeline = attr.Value
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
				var v Asset
				if err := d.DecodeElement(&v, &t); err != nil {
					return err
				}
				// pipelines are set on layout.UnmarshalXML
				c.Assets = append(c.Assets, v)
			}
		}
	}
	return nil
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
				v.pipeline, err = l.pipelines.Get(v.AssetType)
				if err != nil {
					log.Warn(
						fmt.Sprintf("pipeline '%s' not found for asset %s: %s", v.AssetType, v.Name, err),
						log.Tag{Key: log.SrcTag, Value: "layout"},
					)
				}
				l.assets = append(l.assets, v)
			case "Fragment":
				var v Fragment
				if err := d.DecodeElement(&v, &se); err != nil {
					return err
				}
				v.pipeline, err = l.pipelines.Get(v.Pipeline)
				if err != nil {
					log.Warn(
						fmt.Sprintf("pipeline '%s' not found for fragment %s, err: %s", v.Pipeline, v.Name, err),
						log.Tag{Key: log.SrcTag, Value: "layout"},
					)
				}
				l.fragments = append(l.fragments, v)
			case "Content":
				var child Content
				if err := d.DecodeElement(&child, &se); err != nil {
					return err
				}
				for i, a := range child.Assets {
					child.Assets[i].pipeline, err = l.pipelines.Get(a.AssetType)
					if err != nil {
						log.Warn(
							fmt.Sprintf("pipeline '%s' not found for content asset %s, err: %s", a.AssetType, a.Name, err),
							log.Tag{Key: log.SrcTag, Value: "layout"},
						)
					}
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
func (l Layout) Name() string {
	return l.name
}

// MatchContent checks if the given path matches the layout's content
func (l Layout) MatchContent(path string) bool {
	match, err := zglob.Match(l.content.Path, path)
	if err != nil {
		log.Error(fmt.Errorf("error matching layout with path %s: %v", path, err))
		return false
	}

	return match
}

// Parse parses a layout.xml file. The basePath is the root of the project, and path is the path including the name of the file and should be relative to project path
func Parse(basePath string, path string, data []byte, pipeReg *pipeline.Registry, useHotReload bool) (*Layout, error) {
	layout := &Layout{
		basePath:     basePath,
		path:         filepath.Dir(path),
		pipelines:    pipeReg,
		name:         filepath.Base(path),
		useHotReload: useHotReload,
	}
	err := xml.Unmarshal(data, layout)
	if err != nil {
		return nil, err
	}

	return layout, nil
}
