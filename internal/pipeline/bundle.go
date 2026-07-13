package pipeline

import (
	"craeft/internal/utils"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

const (
	pipeGlobalName = "__pipe__"
)

type bundle struct {
	ctx  api.BuildContext
	root *utils.RootFS
}

func newBundle(entryFile string, root *utils.RootFS, pipesRoot *utils.RootFS, debug bool) (*bundle, error) {
	ctx, err := api.Context(api.BuildOptions{
		EntryPoints:   []string{entryFile},
		AbsWorkingDir: root.Path(),
		Format:        api.FormatIIFE,
		GlobalName:    pipeGlobalName,

		Bundle: true,
		Write:  false,

		TreeShaking: api.TreeShakingTrue,

		MinifyIdentifiers: true,
		MinifyWhitespace:  true,
		MinifySyntax:      true,

		Target:    api.ES2020,
		Sourcemap: api.SourceMapInline,
		Supported: map[string]bool{
			"async-await":     false, // transforms async/await → .then() chains
			"async-generator": false, // transforms async generators
			"for-await":       false, // transforms for-await-of loops
			"top-level-await": false, // disallows top-level await
		},
		External:  []string{"kv", "fs", "http", "md", "fasttpl", "tpl", "yaml"},
		NodePaths: []string{filepath.Join(pipesRoot.Path(), "shared", "node_modules")},
		Plugins:   []api.Plugin{pipeRootPlugin(pipesRoot, filepath.Base(root.Path()))},
	})
	if err != nil {
		return nil, fmt.Errorf("esbuild context: %w", err)
	}
	return &bundle{ctx: ctx, root: root}, nil
}

func (b *bundle) build(debug bool) (string, error) {
	result := b.ctx.Rebuild()

	if len(result.Errors) > 0 {
		msgs := api.FormatMessages(result.Errors, api.FormatMessagesOptions{Color: false})
		return "", fmt.Errorf("esbuild: %s", strings.Join(msgs, "; "))
	}
	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("esbuild produced no output")
	}

	out := string(result.OutputFiles[0].Contents)

	if debug {
		// write .dbg.js into the pipe's own folder for inspection — best effort
		_ = b.root.WriteFile("pipeline.dbg.js", []byte(out), 0o644)
	}

	return out, nil
}

func (b *bundle) close() {
	b.ctx.Dispose()
}

func pipeRootPlugin(pipesRoot *utils.RootFS, pipeName string) api.Plugin {
	absPipes := pipesRoot.Path()
	pipeNM := filepath.Join(absPipes, pipeName, "node_modules")
	sharedNM := filepath.Join(absPipes, "shared", "node_modules")
	sep := string(filepath.Separator)

	within := func(base, p string) bool {
		rel, err := filepath.Rel(base, p)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+sep)
	}

	return api.Plugin{
		Name: "sandbox",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: `.*`},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					if args.PluginData != nil {
						return api.OnResolveResult{}, nil
					}

					probe := build.Resolve(args.Path, api.ResolveOptions{
						ResolveDir: args.ResolveDir,
						Importer:   args.Importer,
						Kind:       args.Kind,
						PluginData: true,
					})
					if len(probe.Errors) > 0 {
						return api.OnResolveResult{Errors: probe.Errors}, nil
					}
					if probe.External {
						return api.OnResolveResult{}, nil
					}

					if !filepath.IsAbs(probe.Path) {
						return api.OnResolveResult{}, nil
					}

					rel, err := filepath.Rel(absPipes, probe.Path)
					if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+sep) {
						return api.OnResolveResult{Errors: []api.Message{{
							Text: fmt.Sprintf("import %q escapes pipes root (resolved to %q)",
								args.Path, probe.Path),
						}}}, nil
					}

					if strings.Contains(probe.Path, sep+"node_modules"+sep) &&
						!within(pipeNM, probe.Path) && !within(sharedNM, probe.Path) {
						return api.OnResolveResult{Errors: []api.Message{{
							Text: fmt.Sprintf("package %q resolved to disallowed node_modules: %q",
								args.Path, probe.Path),
						}}}, nil
					}

					if _, err := pipesRoot.Stat(rel); err != nil {
						return api.OnResolveResult{Errors: []api.Message{{
							Text: fmt.Sprintf("not accessible in sandbox %q: %v", rel, err),
						}}}, nil
					}

					return api.OnResolveResult{}, nil
				})
		},
	}
}
