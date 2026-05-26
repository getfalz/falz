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

func newBundle(entryFile string, root *utils.RootFS, debug bool) (*bundle, error) {

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
		External: []string{"kv", "fs", "http", "md", "fasttpl", "tpl"},
		Plugins: []api.Plugin{
			pipeRootPlugin(root),
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

func pipeRootPlugin(root *utils.RootFS) api.Plugin {
	absRoot := root.Path()
	return api.Plugin{
		Name: "fsroot",
		Setup: func(build api.PluginBuild) {
			build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "file"},
				func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					absPath := args.Path
					if !filepath.IsAbs(absPath) {
						return api.OnLoadResult{}, fmt.Errorf("fsroot: expected absolute resolved path, got %q", absPath)
					}
					rel, err := filepath.Rel(absRoot, absPath)
					if err != nil {
						return api.OnLoadResult{}, fmt.Errorf("fsroot: rel %q: %w", absPath, err)
					}
					if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
						return api.OnLoadResult{Errors: []api.Message{{
							Text: fmt.Sprintf("fsroot: attempted to load file outside pipe directory: %q", absPath),
						}}}, nil
					}
					data, err := root.ReadFile(rel)
					if err != nil {
						return api.OnLoadResult{}, fmt.Errorf("fsroot: read %q: %w", rel, err)
					}
					contents := string(data)
					return api.OnLoadResult{
						Contents:   &contents,
						Loader:     loaderForExt(filepath.Ext(absPath)),
						ResolveDir: filepath.Dir(absPath),
					}, nil
				})
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
