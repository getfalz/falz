package pipeline

import (
	"fmt"
	"os"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

type bundle struct {
	ctx api.BuildContext
}

func newBundle(entryFile, workDir string, debug bool) (*bundle, error) {
	ctx, err := api.Context(api.BuildOptions{
		EntryPoints:      []string{entryFile},
		AbsWorkingDir:    workDir,
		Format:           api.FormatCommonJS,
		Bundle:           true,
		Write:            false,
		TreeShaking:      api.TreeShakingFalse,
		MinifyWhitespace: false,
		MinifySyntax:     false,
		Target:           api.ES2020,
		Sourcemap:        api.SourceMapInline,
		Supported: map[string]bool{
			"async-await":     false, // transforms async/await → .then() chains
			"async-generator": false, // transforms async generators
			"for-await":       false, // transforms for-await-of loops
			"top-level-await": false, // disallows top-level await
		},
		External: []string{"kv", "fs", "http", "md", "fasttpl", "tpl"},
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
