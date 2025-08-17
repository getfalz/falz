package layout

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/mattn/go-zglob"
	"github.com/valyala/fasttemplate"
)

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
	path         string
	pipelines    *pipeline.Registry
	root         *os.Root
	useHotReload bool

	// Children
	fragments []Fragment
	contents  []Content
	assets    []Asset

	Tpl     string `xml:"tpl,attr"`
	TplPath string `xml:"path,attr"`
}

type Fragment struct {
	pipeline pipeline.PipelineScript
	root     *os.Root

	XMLName xml.Name `xml:"Fragment"`
	Name    string   `xml:"name,attr"`
	Source  string   `xml:"src,attr"`
	Pipe    string   `xml:"pipe,attr"`
}

type Content struct {
	XMLName xml.Name `xml:"Content"`
	Name    string   `xml:"name,attr"`
	Path    string   `xml:"path,attr"`
	Pipe    string   `xml:"pipe,attr"`
	Assets  []Asset  `xml:"Assets"`
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
func (f Fragment) Render(ctx context.Context) (string, string, error) {
	basePath := getValue(ctx, ctxBasePath)
	renderPath := getValue(ctx, ctxRenderPath)

	files, err := utils.FindFile(basePath, renderPath, f.Source)
	if err != nil {
		return "", "", err
	}

	if len(files) == 0 {
		return "", "", fmt.Errorf("no files found for fragment %s", f.Source)
	}

	var content []byte
	fragment := filepath.ToSlash(filepath.Join(basePath, files[0]))
	// TODO use schemeRoot.Open()
	content, err = os.ReadFile(fragment)
	if err != nil {
		return "", "", err
	}

	vars := pipeline.NewVars("")
	// TODO pass global context
	result, err := f.pipeline.Transform(ctx, content, vars)
	if err != nil {
		return "", "", fmt.Errorf("pipeline %s: %w", f.pipeline.Config().PipelineName, err)
	}

	if len(result) == 0 {
		return "", "", fmt.Errorf("pipeline '%s' for fragment '%s' returned an empty result", f.pipeline.Config().PipelineName, f.Name)
	}
	if len(result) > 1 {
		log.Warn(fmt.Sprintf("Fragments pipeline %s returned more than one result. Using the first one", f.pipeline.Config().PipelineName), log.Tag{Key: log.PipelineTag, Value: f.Name})
	}

	return result[0]["result"], fragment, nil
}

// func getValue(ctx context.Context, key contextKey) string {
// 	// Use the key to get the value from the context
// 	if val, ok := ctx.Value(key).(string); ok {
// 		return val
// 	}
// 	return ""
// }

// Content struct loads Markdown, converts it to HTML, and renders it
func (c Content) Render(ctx context.Context) (string, error) {
	return fmt.Sprintf("{{%s}}", c.Name), nil
}

// TODO: deprecate? this is css/js specific. maybe only return the new path
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
func (l *Layout) RenderWithDeps(base string, schemeRoot *os.Root) (rendered string, deps []string, err error) {
	// func (l *Layout) Render(path string) (string, error) {
	// TODO render websocket script only when the development flag is set
	// but find a way to define the variables placeholders. maybe use fasttemplate?

	var tmpl []byte
	if l.Tpl != "" {
		tmpl = []byte(l.Tpl)
	} else {
		tplPaths, err := utils.FindFile(l.basePath, base, l.TplPath)
		if err != nil {
			return "", nil, fmt.Errorf("can't find layout template: %w", err)
		}

		p := filepath.ToSlash(tplPaths[0])
		f, err := schemeRoot.Open(p)
		if err != nil {
			return "", nil, err
		}
		b, rerr := io.ReadAll(f)
		_ = f.Close()
		if rerr != nil {
			return "", nil, rerr
		}
		tmpl = b
		deps = append(deps, p)

	}

	ctx := context.WithValue(context.Background(), ctxBasePath, l.basePath)
	ctx = context.WithValue(ctx, ctxRenderPath, base)
	ctx = context.WithValue(ctx, ctxLayoutPath, l.path)

	tplVariables := make(map[string]any)
	for _, asset := range l.assets {
		output, err := asset.Render(ctx)
		if err != nil {
			return "", nil, err
		}
		tplVariables[asset.Name] = output
	}

	for _, c := range l.contents {
		content, err := c.Render(ctx)
		if err != nil {
			return "", nil, err
		}
		tplVariables[c.Name] = content
	}

	for _, f := range l.fragments {
		output, fragmentPath, err := f.Render(ctx)
		if err != nil {
			return "", nil, err
		}
		tplVariables[f.Name] = output
		deps = append(deps, fragmentPath)
	}

	if l.useHotReload {
		tplVariables["hotreload"] = wsClient
	}

	log.Trace(fmt.Sprintf("Rendering layout %q with variables: %v", l.Tpl, tplVariables))
	result := parseTpl(string(tmpl), tplVariables)

	return result, deps, nil
}

func (l Layout) Validate() error {
	if l.Tpl == "" && l.TplPath == "" {
		return errors.New("element <Layout>: layout requires Tpl or TplPath")
	}
	return nil
}

func (a Asset) Validate() error {
	if a.Name == "" {
		return errors.New("element <Asset>: missing required field: name")
	}
	if a.Source == "" {
		return errors.New("element <Asset>: missing required field: src")
	}
	return nil
}

func (f Fragment) Validate() error {
	if f.Name == "" {
		return errors.New("element <Fragment>: missing required field: name")
	}
	if f.Source == "" {
		return errors.New("element <Fragment>: missing required field: src")
	}
	return nil
}

func (c Content) Validate() error {
	if c.Name == "" {
		return errors.New("element <Content>: missing required field: name")
	}
	if c.Path == "" {
		return errors.New("element <Content>: missing required field: path")
	}
	return nil
}

func (l *Layout) Fragments() []string {
	var fragments []string

	for _, f := range l.fragments {
		fragments = append(fragments, f.Source)
	}

	return fragments
}

func (l Layout) Contents() []Content {
	return l.contents
}

func (l Layout) Assets() []Asset {
	return l.assets
}

// Path is the relative path of the layout to the project root folder.
func (l Layout) Path() string {
	return normalizeRelFile(l.path)
}

func (c Content) MatchContent(path string) bool {
	match, err := zglob.Match(c.Path, path)
	if err != nil {
		// TODO(now) why is an error needed?. this can be refactor to a simple break
		log.Error(fmt.Errorf("error matching layout with path %s: %v", path, err))
		return false
	}
	return match
}

// MatchContent checks if the given path matches the layout's content
func (l Layout) MatchContent(path string) bool {
	for _, c := range l.contents {
		if c.MatchContent(path) {
			return true
		}
	}

	return false
}

func normalizeRelFile(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return p
}

// Parse parses a layout.xml file. The basePath is the root of the project, and path is the path including the name of the file and should be relative to project path
func Parse(basePath string, path string, data []byte, pipeReg *pipeline.Registry, useHotReload bool, schemeRoot *os.Root) (*Layout, error) {

	l := &Layout{
		basePath:     basePath,
		path:         normalizeRelFile(path),
		pipelines:    pipeReg,
		useHotReload: useHotReload,
		root:         schemeRoot,
	}
	err := xml.Unmarshal(data, l)
	if err != nil {
		return nil, err
	}

	log.TTrace(log.Layout).
		Str("layout_xml", l.path).
		Bool("hotreload", useHotReload).
		Msg("layout parsed")

	return l, nil
}
