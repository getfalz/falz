package layout

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/mattn/go-zglob"
	"github.com/valyala/fasttemplate"
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

type RenderCtx struct {
	Pipelines *pipeline.Registry
	FS        *utils.RootFS
}

func (f *Fragment) Render(ctx context.Context, rctx RenderCtx) (string, string, error) {
	files, err := rctx.FS.FindFile(f.base, f.Source)
	if err != nil {
		return "", "", err
	}

	fragmentPath := filepath.ToSlash(files[0])
	log.TTrace(log.Layout).
		Str("fragment", f.Name).
		Str("src", f.Source).
		Str("file", fragmentPath).Msg("fragment: resolved")

	content, err := fs.ReadFile(rctx.FS.FS(), fragmentPath)
	if err != nil {
		return "", "", fmt.Errorf("fragment %q: can't read file %q: %w", f.Name, fragmentPath, err)
	}

	vars := pipeline.Vars{}
	pipe, err := rctx.Pipelines.Get(f.Pipe)
	if err != nil {
		return "", "", fmt.Errorf("fragment %q: pipe %q not found: %w", f.Name, pipe.Config().PipelineName, err)
	}
	// TODO set vars
	// vars.Set("vars", t.P.Config.Variables)
	// vars.Set("assets", f.)
	result, err := pipe.Transform(ctx, content, vars)
	if err != nil {
		return "", "", fmt.Errorf("fragment %q: pipe %q couldn't transform: %w", f.Name, pipe.Config().PipelineName, err)
	}

	log.DDebug(log.Layout).
		Str("fragment", f.Name).
		Str("src", fragmentPath).
		Msg("fragment rendered")

	return result, fragmentPath, nil
}

func (c Content) Render(ctx context.Context) string {
	return fmt.Sprintf("{{%s}}", c.Name)
}

// Render layout. The fragments will be searched first in the given base path
func (l *Layout) Render(ctx context.Context, rctx RenderCtx, base string) (rendered string, deps []string, err error) {

	log.TTrace(log.Layout).
		Str("layout_rel", l.path).
		Str("base", filepath.ToSlash(base)).
		Bool("hotreload", l.useHotReload).
		Msg("layout render begin")

	if cerr := ctx.Err(); cerr != nil {
		log.EError(log.Layout).Err(cerr).Str("layout_rel", l.path).Msg("render canceled (pre)")
		return "", nil, cerr
	}

	var tmpl []byte
	if l.tpl != "" {
		tmpl = []byte(l.tpl)

		log.DDebug(log.Layout).
			Str("layout_rel", l.path).
			Int("inline_len", len(tmpl)).
			Msg("using inline layout template")
	} else {
		tplPaths, err := rctx.FS.FindFile(base, l.TplPath)
		if err != nil {
			return "", nil, fmt.Errorf("layout %q: find template with base %q and path %q: %w", l.path, base, l.TplPath, err)
		}

		tplPath := filepath.ToSlash(tplPaths[0])
		tplFile, err := fs.ReadFile(rctx.FS.FS(), tplPath)
		if err != nil {
			return "", nil, fmt.Errorf("layout %q: read template file %q: %w", l.path, tplPath, err)
		}
		tmpl = tplFile
		deps = append(deps, tplPath)

		log.DDebug(log.Layout).
			Str("layout_rel", l.path).
			Str("tpl_file", tplPath).
			Int("bytes", len(tmpl)).
			Msg("loaded layout template file")
	}

	tplVariables := make(map[string]any)

	for _, c := range l.contents {
		tplVariables[c.Name] = c.Render(ctx)
	}

	for _, f := range l.fragments {
		output, fragmentPath, err := f.Render(ctx, rctx)
		if err != nil {
			return "", nil, fmt.Errorf("layout %q: render fragment %q with src %q: %w", l.path, f.Name, f.Source, err)
		}
		tplVariables[f.Name] = output
		deps = append(deps, fragmentPath)

		log.TTrace(log.Layout).
			Str("layout_rel", l.path).
			Str("fragment", f.Name).
			Str("src", f.Source).
			Str("dep", fragmentPath).
			Msg("fragment rendered")
	}

	// add reserved variables
	if l.useHotReload {
		tplVariables["hotreload"] = wsClient
	}

	log.DDebug(log.Layout).
		Str("layout_rel", l.path).
		Int("assets", len(l.assets)).
		Int("contents", len(l.contents)).
		Int("fragments", len(l.fragments)).
		Msg("rendering layout with variables")

	// builds layout and repace placeholders for fragments
	t, err := fasttemplate.NewTemplate(string(tmpl), "{{", "}}")
	if err != nil {
		return "", nil, fmt.Errorf("layout %q: parse template: %w", l.path, err)
	}
	result := t.ExecuteString(tplVariables)

	log.TTrace(log.Layout).
		Str("layout_rel", l.path).
		Int("deps", len(deps)).
		Int("vars", len(tplVariables)).
		Msg("layout render done")

	return result, deps, nil
}

func (l Layout) Validate() error {
	if l.tpl == "" && l.TplPath == "" {
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
	if a.To == "" {
		return errors.New("element <Asset>: missing required field: to")
	}
	return nil
}

func (f *Fragment) Validate() error {
	if f.Name == "" {
		return errors.New("element <Fragment>: missing required field: name")
	}
	if f.Source == "" {
		return errors.New("element <Fragment>: missing required field: src")
	}
	return nil
}

func (c *Content) Validate() error {
	if c.Name == "" {
		return errors.New("element <Content>: missing required field: name")
	}
	if c.Source == "" {
		return errors.New("element <Content>: missing required field: src")
	}
	if c.To == "" {
		return errors.New("element <Content>: missing required field: to")
	}
	return nil
}

func (l *Layout) Fragments() []Fragment {
	out := make([]Fragment, len(l.fragments))
	copy(out, l.fragments)
	return out
}

func (l *Layout) Contents() []Content {
	out := make([]Content, len(l.contents))
	copy(out, l.contents)
	return out
}

func (l *Layout) Assets() []Asset {
	out := make([]Asset, len(l.assets))
	copy(out, l.assets)
	return out
}

// Path is the relative path of the layout to the project root folder.
func (l *Layout) Path() string {
	return normalizeRelFile(l.path)
}

func (c *Content) Assets() []Asset {
	return c.assetElms
}

// MatchContent checks if the given path matches the layout's content
func (l *Layout) MatchContent(path string) bool {
	for _, c := range l.contents {
		if c.MatchContent(path) {
			return true
		}
	}

	return false
}

func (c *Content) MatchContent(path string) bool {
	match, err := zglob.Match(c.Source, path)
	if err != nil {
		return false
	}
	return match
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
func Parse(path string, data []byte, useHotReload bool) (*Layout, error) {

	relPath := normalizeRelFile(path)

	l := &Layout{
		path:         relPath,
		useHotReload: useHotReload,
	}
	err := xml.Unmarshal(data, l)
	if err != nil {
		return nil, err
	}

	if err := l.Validate(); err != nil {
		return nil, err
	}

	log.TTrace(log.Layout).
		Str("layout_xml", l.path).
		Bool("hotreload", useHotReload).
		Int("assets", len(l.assets)).
		Int("contents", len(l.contents)).
		Int("fragments", len(l.fragments)).
		Msg("layout parsed")

	return l, nil
}
