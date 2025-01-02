package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/bep/godartsass/v2"
	"github.com/evanw/esbuild/pkg/api"
)

type SassPlugin struct {
	transpiler *godartsass.Transpiler
	mu         sync.Mutex

	enableSourceMap bool
	outputStyle     godartsass.OutputStyle
	cssSyntax       godartsass.SourceSyntax
	themePath       string
}

func NewSassPlugin(config *CraeftConfig) *SassPlugin {
	t, err := godartsass.Start(godartsass.Options{
		DartSassEmbeddedFilename: config.StylesConfig.SassPath,
	})

	var outputStyle godartsass.OutputStyle
	if config.StylesConfig.Compressed {
		outputStyle = godartsass.OutputStyleCompressed
	} else {
		outputStyle = godartsass.OutputStyleExpanded
	}

	var cssSyntax godartsass.SourceSyntax
	switch config.StylesConfig.Parser {
	case "scss":
		cssSyntax = godartsass.SourceSyntaxCSS
	case "indented":
		cssSyntax = godartsass.SourceSyntaxSASS
	default:
		cssSyntax = godartsass.SourceSyntaxSCSS
	}

	if err != nil {
		return nil
	}
	return &SassPlugin{
		transpiler:      t,
		enableSourceMap: config.StylesConfig.SourceMap,
		outputStyle:     outputStyle,
		cssSyntax:       cssSyntax,
		themePath:       config.ThemePath,
	}
}

func (sp *SassPlugin) Plugin() api.Plugin {
	return api.Plugin{
		Name: "sass-plugin",
		Setup: func(build api.PluginBuild) {
			build.OnLoad(api.OnLoadOptions{Filter: `\.s[ac]ss$`}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {

				styles, err := os.ReadFile(args.Path)
				if err != nil {
					return api.OnLoadResult{}, err
				}

				sp.mu.Lock()

				result, err := sp.transpiler.Execute(godartsass.Args{
					// URL:                     args.Path,
					Source:                  string(styles),
					EnableSourceMap:         sp.enableSourceMap,
					SourceMapIncludeSources: true,
					OutputStyle:             sp.outputStyle,
					SourceSyntax:            sp.cssSyntax,
					IncludePaths:            []string{sp.themePath},
				})
				sp.mu.Unlock()

				if err != nil {
					return api.OnLoadResult{}, fmt.Errorf("error compiling Sass: %v", err)
				}

				return api.OnLoadResult{
					Contents:   &result.CSS,
					Loader:     api.LoaderCSS,
					ResolveDir: filepath.Dir(args.Path),
				}, nil
			})
		},
	}
}
