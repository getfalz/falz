package layout

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/valyala/fasttemplate"
)

type RenderCtx struct {
	Pipelines *pipeline.Registry
	FS        *utils.RootFS
}

func (f *Fragment) Render(ctx context.Context, rctx RenderCtx, compileVars map[string]string) ([]byte, string, error) {
	files, err := rctx.FS.FindFile(f.base, f.Source)
	if err != nil {
		return nil, "", err
	}

	fragmentPath := filepath.ToSlash(files[0])

	content, err := fs.ReadFile(rctx.FS.FS(), fragmentPath)
	if err != nil {
		return nil, "", fmt.Errorf("fragment %q: can't read file %q: %w", f.Name, fragmentPath, err)
	}

	pipe, err := rctx.Pipelines.Get(f.Pipe)
	if err != nil {
		return nil, "", fmt.Errorf("fragment %q: pipe %q not found: %w", f.Name, f.Pipe, err)
	}
	vars := pipeline.Vars{
		"file": pipeline.NewFileInfo(fragmentPath, filepath.Dir(fragmentPath)),
	}
	for k, v := range compileVars {
		vars[k] = v
	}
	result, err := pipe.Transform(ctx, content, vars)
	if err != nil {
		return nil, "", fmt.Errorf("fragment %q: pipe %q couldn't transform: %w", f.Name, pipe.Config().PipelineName, err)
	}

	log.TTrace(log.Layout).
		Str("fragment", f.Name).
		Str("src", fragmentPath).
		Str("file", fragmentPath).
		Msg("fragment rendered")

	return result.Content, fragmentPath, nil
}

func (c Content) Render(ctx context.Context) string {
	return fmt.Sprintf("{{%s}}", c.Name)
}

// Render layout. The fragments will be searched first in the given base path
func (l *Layout) Render(ctx context.Context, rctx RenderCtx, base string, compileVars map[string]string) (rendered []byte, deps []string, err error) {

	if cerr := ctx.Err(); cerr != nil {
		log.EError(log.Layout).Err(cerr).Str("layout_rel", l.path).Msg("render canceled (pre)")
		return nil, nil, cerr
	}

	var tmpl []byte
	if l.tpl != "" {
		tmpl = []byte(l.tpl)
	} else {
		tplPaths, err := rctx.FS.FindFile(base, l.TplPath)
		if err != nil {
			return nil, nil, fmt.Errorf("layout %q: find template with base %q and path %q: %w", l.path, base, l.TplPath, err)
		}

		tplPath := filepath.ToSlash(tplPaths[0])
		tplFile, err := fs.ReadFile(rctx.FS.FS(), tplPath)
		if err != nil {
			return nil, nil, fmt.Errorf("layout %q: read template file %q: %w", l.path, tplPath, err)
		}
		tmpl = tplFile
		deps = append(deps, tplPath)
	}

	tplVariables := make(map[string]any)

	for k, v := range compileVars {
		tplVariables[k] = v
	}

	for _, c := range l.contents {
		tplVariables[c.Name] = c.Render(ctx)
	}

	for _, f := range l.fragments {
		output, fragmentPath, err := f.Render(ctx, rctx, compileVars)
		if err != nil {
			return nil, nil, fmt.Errorf("layout %q: render fragment %q with src %q: %w", l.path, f.Name, f.Source, err)
		}
		tplVariables[f.Name] = output
		deps = append(deps, fragmentPath)
	}

	// builds layout and replaces placeholders for fragments
	t, err := fasttemplate.NewTemplate(string(tmpl), "{{", "}}")
	if err != nil {
		return nil, nil, fmt.Errorf("layout %q: parse template: %w", l.path, err)
	}

	var buf bytes.Buffer
	if _, err = t.Execute(&buf, tplVariables); err != nil {
		return nil, nil, fmt.Errorf("layout %q: execute template: %w", l.path, err)
	}

	lo := log.TTrace(log.Layout).
		Str("layout_rel", l.path).
		Str("base", filepath.ToSlash(base))
	if l.tpl != "" {
		lo.Int("inline_len", len(l.tpl))
	} else {
		lo.Str("tpl_file", deps[0])
	}

	lo.Int("assets", len(l.assets)).
		Int("contents", len(l.contents)).
		Int("fragments", len(l.fragments)).
		Msg("layout template rendered")

	return buf.Bytes(), deps, nil
}

func (l *Layout) CollectDeps(ctx context.Context, rctx RenderCtx, base string) ([]string, error) {
	var deps []string
	if l.TplPath != "" {
		paths, err := rctx.FS.FindFile(base, l.TplPath)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", l.TplPath, err)
		}
		if len(paths) > 0 {
			deps = append(deps, filepath.ToSlash(paths[0]))
		}
	}
	for _, f := range l.fragments {
		files, err := rctx.FS.FindFile(f.base, f.Source)
		if err != nil {
			return nil, fmt.Errorf("fragment %q: %w", f.Source, err)
		}
		if len(files) > 0 {
			deps = append(deps, filepath.ToSlash(files[0]))
		}
	}
	return deps, nil
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
	// TODO only check if in content
	// if strings.ContainsAny(a.Source, "/\\") {
	// 	return fmt.Errorf("element <Content>: field 'src' must not contain path separators (got %q)", a.Source)
	// }
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
	if strings.ContainsAny(c.Source, "/\\") {
		return fmt.Errorf("element <Content>: field 'src' must not contain path separators (got %q)", c.Source)
	}
	return nil
}

func (c *Content) Vars() map[string]string {
	return c.vars
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

func (c *Content) Assets() []Asset {
	return c.assetElms
}

func (l *Layout) buildOutputs() error {
	l.outputs = make(map[string]*fasttemplate.Template)

	for _, c := range l.Contents() {
		tpl, err := fasttemplate.NewTemplate(c.To, "{", "}")
		if err != nil {
			log.Warn(fmt.Sprintf("failed to parse content output template for content %q: %v. This content will be ignored.", c.Name, err))
			continue
		}
		l.outputs[c.Name] = tpl

		for _, asset := range c.Assets() {
			assetTpl, err := fasttemplate.NewTemplate(asset.To, "{", "}")
			if err != nil {
				log.Warn(fmt.Sprintf("failed to parse content asset output template for asset %q: %v. This asset will be ignored.", asset.Name, err))
				continue
			}
			l.outputs[asset.Name] = assetTpl
		}
	}

	for _, c := range l.Assets() {
		tpl, err := fasttemplate.NewTemplate(c.To, "{", "}")
		if err != nil {
			log.Warn(fmt.Sprintf("failed to parse asset output template for asset %q: %v. This asset will be ignored.", c.Name, err))
			continue
		}
		l.outputs[c.Name] = tpl
	}
	return nil
}

var hasHashTag = regexp.MustCompile(`^HASH:(\d+)$`)

type OutputEntry struct {
	SourcePath string
	Pipeline   string
	Hash       string // may be "" if not computed yet
	BaseDir    string
	Alias      string
}

func (l *Layout) ContentOutput(contentName string, entry OutputEntry) (string, error) {
	tpl, ok := l.outputs[contentName]
	if !ok {
		return "", fmt.Errorf("content %q not found in layout", contentName)
	}
	if tpl == nil {
		return "", fmt.Errorf("content %q has no output template", contentName)
	}

	name := filepath.Base(entry.SourcePath)
	extWithDot := filepath.Ext(name)
	filename := strings.TrimSuffix(name, extWithDot)
	ext := strings.TrimPrefix(extWithDot, ".")

	// TODO don't panic on template parse
	// e.g. panic: unexpected error: unknown placeholder: FILENAM
	output := tpl.ExecuteFuncString(func(w io.Writer, tag string) (int, error) {
		switch tag {
		case "FILENAME":
			return w.Write([]byte(filename))
		case "EXT":
			return w.Write([]byte(ext))
		case "BASENAME":
			return w.Write([]byte(name))
		case "ALIAS":
			return w.Write([]byte(entry.Alias))
		case "PIPE":
			return w.Write([]byte(entry.Pipeline))
		case "HASH":
			if entry.Hash == "" {
				return 0, fmt.Errorf("HASH not available yet")
			}
			return w.Write([]byte(entry.Hash))
		default:
			// HASH:N ?
			if m := hasHashTag.FindStringSubmatch(tag); len(m) == 2 {
				if entry.Hash == "" {
					return 0, fmt.Errorf("HASH not available yet")
				}

				n, err := strconv.Atoi(m[1])
				if err != nil || n <= 0 {
					return 0, fmt.Errorf("invalid HASH length: %q", m[1])
				}
				if n > len(entry.Hash) {
					n = len(entry.Hash)
				}
				return w.Write([]byte(entry.Hash[:n]))
			}

			return fmt.Fprintf(w, "[unknown tag %q]", tag)
		}
	})

	return filepath.Join(entry.BaseDir, utils.NormPath(output)), nil
}

// OutputTemplate returns the output-path template for a given content/asset
// name. Independent of vars — safe to call without ever rendering.
func (l *Layout) OutputTemplate(name string) (*fasttemplate.Template, bool) {
	tpl, ok := l.outputs[name]
	return tpl, ok
}

// Parse parses a layout.xml file. The basePath is the root of the project, and path is the path including the name of the file and should be relative to project path
func Parse(path string, data []byte) (*Layout, error) {

	relPath := utils.NormPath(path)

	l := &Layout{
		path: relPath,
	}
	err := xml.Unmarshal(data, l)
	if err != nil {
		return nil, err
	}

	if err := l.Validate(); err != nil {
		return nil, err
	}

	if err := l.buildOutputs(); err != nil {
		return nil, err
	}

	log.TTrace(log.Layout).
		Str("layout", l.path).
		Int("assets", len(l.assets)).
		Int("contents", len(l.contents)).
		Int("fragments", len(l.fragments)).
		Msg("layout xml parsed")

	return l, nil
}
