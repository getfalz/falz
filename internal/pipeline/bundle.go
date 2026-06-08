package pipeline

import (
	"craeft/internal/utils"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

const (
	pipeGlobalName = "__pipe__"
)

type bundle struct {
	ctx api.BuildContext
}

func newBundle(entryFile string, root *utils.RootFS, sharedRoot *utils.RootFS, debug bool) (*bundle, error) {

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
		External:  []string{"kv", "fs", "http", "md", "fasttpl", "tpl"},
		NodePaths: []string{filepath.Join(sharedRoot.Path(), "node_modules")},
		Plugins: []api.Plugin{
			sharedSourcePlugin(sharedRoot),
			pipeRootPlugin(root, sharedRoot),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("esbuild context: %w", err)
	}
	return &bundle{ctx: ctx}, nil
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
		// write .dbg.js alongside entry for inspection — best effort
		_ = os.WriteFile("pipeline.dbg.js", []byte(out), 0o644)
	}

	return out, nil
}

func (b *bundle) close() {
	b.ctx.Dispose()
}

func sharedSourcePlugin(sharedRoot *utils.RootFS) api.Plugin {
	return api.Plugin{
		Name: "shared-source",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: `^@shared/`},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{
						Path:      strings.TrimPrefix(args.Path, "@shared/"),
						Namespace: "shared",
					}, nil
				})
		},
	}
}

func pipeRootPlugin(pipeRoot *utils.RootFS, sharedRoot *utils.RootFS) api.Plugin {
	absPipe := pipeRoot.Path()
	absShared := sharedRoot.Path()

	within := func(base, abs string) bool {
		rel, err := filepath.Rel(base, abs)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}

	handler := func(args api.OnLoadArgs) (api.OnLoadResult, error) {
		var (
			data       []byte
			err        error
			resolveDir string
		)

		switch args.Namespace {
		case "file":
			if !within(absPipe, args.Path) && !within(absShared, args.Path) {
				return api.OnLoadResult{Errors: []api.Message{{
					Text: fmt.Sprintf("fsroot: file outside allowed directories: %q", args.Path),
				}}}, nil
			}
			root := pipeRoot
			base := absPipe
			if within(absShared, args.Path) {
				root = sharedRoot
				base = absShared
			}
			rel, _ := filepath.Rel(base, args.Path)
			data, err = root.ReadFile(rel)
			resolveDir = filepath.Dir(args.Path)

		case "shared":
			data, err = sharedRoot.ReadFile(args.Path)
			resolveDir = filepath.Join(absShared, filepath.Dir(args.Path))
		}

		if err != nil {
			return api.OnLoadResult{}, fmt.Errorf("fsroot [%s] read %q: %w", args.Namespace, args.Path, err)
		}
		contents := string(data)
		return api.OnLoadResult{
			Contents:   &contents,
			Loader:     loaderForExt(filepath.Ext(args.Path)),
			ResolveDir: resolveDir,
		}, nil
	}

	return api.Plugin{
		Name: "fsroot",
		Setup: func(build api.PluginBuild) {
			build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "file"}, handler)
			build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "shared"}, handler)
		},
	}
}

func loaderForExt(ext string) api.Loader {
	switch ext {
	case ".js", ".mjs", ".cjs":
		return api.LoaderJS
	case ".ts", ".tsx", ".mts", ".cts":
		return api.LoaderTS
	case ".json", ".jsonc":
		return api.LoaderJSON

	case ".txt", ".text",
		".md", ".markdown",
		".html", ".htm",
		".css",
		".svg",
		".xml",
		".csv",
		".yaml", ".yml",
		".toml":
		return api.LoaderText
	default:
		return api.LoaderBinary
	}
}
