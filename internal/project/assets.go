package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bep/godartsass/v2"
	"github.com/evanw/esbuild/pkg/api"
	log "github.com/sirupsen/logrus"
)

type Assets struct {
	config  *CraeftConfig
	trp     *godartsass.Transpiler
	hasSass bool
}

func NewAssets(config *CraeftConfig) *Assets {

	t, err := godartsass.Start(godartsass.Options{
		DartSassEmbeddedFilename: config.StylesConfig.SassPath,
	})

	var hasSass bool
	if err != nil {
		hasSass = false
		log.Errorf("Failed to start sass compiler: %v", err)
	}

	return &Assets{
		config:  config,
		trp:     t,
		hasSass: hasSass,
	}
}

func (a *Assets) withDartSass(styles []byte) (string, error) {
	var outputStyle godartsass.OutputStyle
	if a.config.StylesConfig.Compressed {
		outputStyle = godartsass.OutputStyleCompressed
	} else {
		outputStyle = godartsass.OutputStyleExpanded
	}

	var cssSyntax godartsass.SourceSyntax
	switch a.config.StylesConfig.Parser {
	case "scss":
		cssSyntax = godartsass.SourceSyntaxCSS
	case "indented":
		cssSyntax = godartsass.SourceSyntaxSASS
	default:
		cssSyntax = godartsass.SourceSyntaxSCSS
	}

	// TODO test if imported files can be outside of the project. if so,
	// make a custom importer (interface ImportResolver)
	result, err := a.trp.Execute(godartsass.Args{
		Source:                  string(styles),
		EnableSourceMap:         a.config.StylesConfig.SourceMap,
		SourceMapIncludeSources: true,
		OutputStyle:             outputStyle,
		SourceSyntax:            cssSyntax,
		IncludePaths:            []string{a.config.ThemePath},
	})
	if err != nil {
		return "", err
	}

	return result.CSS, nil
}

// TODO implement a sass plugin for esbuild
// with the plugin, we could use esbuild for css and js. and use Rebuild
func (a Assets) withEsBuild(styles []byte) (string, error) {
	// esbuild treats css like garbage in, garbage out.
	// but it would compress the output. no imports
	log.Infof("[SASS] Using esbuild css loader.")
	config := api.TransformOptions{
		Loader: api.LoaderCSS,
		// Sourcefile: fileName,
	}
	if a.config.StylesConfig.SourceMap {
		config.Sourcemap = api.SourceMapInline
	}
	if a.config.StylesConfig.Compressed {
		config.MinifyWhitespace = true
		config.MinifyIdentifiers = true
		config.MinifySyntax = true
	}
	result := api.Transform(string(styles), config)
	return string(result.Code), nil
}

func (a Assets) Close() {
	if a.hasSass {
		a.trp.Close()
	}
}

func (a *Assets) BuildCSS(base, fileName string) error {
	cssPath := filepath.Join(a.config.ThemePath, base, fileName)
	log.Debugf("Processing css: %s", cssPath)

	styles, err := os.ReadFile(cssPath)
	if err != nil {
		return err
	}

	var cssResult string
	if a.hasSass {
		cssResult, err = a.withDartSass(styles)
	} else {
		cssResult, err = a.withEsBuild(styles)
	}
	if err != nil {
		return err
	}

	// write css file
	targetDir := filepath.Join(a.config.OutputDirectory, base)
	// TODO handle suffix correctly.
	outputFile := filepath.Join(targetDir, strings.TrimSuffix(fileName, ".scss")+".css")
	err = os.WriteFile(outputFile, []byte(cssResult), 0644)
	if err != nil {
		log.Fatalf("Failed to write css file: %s", err)
		return err
	}

	log.Debugf("Processed css: %s -> %s\n", cssPath, outputFile)
	return nil
}

func (a *Assets) BuildJS(base, fileName string) error {
	jsPath := filepath.Join(a.config.ThemePath, base, fileName)

	log.Debugf("Processing js: %s", jsPath)
	scripts, err := os.ReadFile(jsPath)
	if err != nil {
		return err
	}

	config := api.BuildOptions{
		Bundle:   true,
		Write:    false,
		Platform: api.PlatformBrowser,
		Stdin: &api.StdinOptions{
			Contents:   string(scripts),
			ResolveDir: a.config.ThemePath,
			Sourcefile: fileName,
		},
	}
	if a.config.ScriptsConfig.SourceMap {
		config.Sourcemap = api.SourceMapInline
	}
	if a.config.ScriptsConfig.Compressed {
		config.MinifyWhitespace = true
		config.MinifyIdentifiers = true
		config.MinifySyntax = true
	}
	result := api.Build(config)

	if len(result.Errors) > 0 {
		return fmt.Errorf("[esbuid] Found errors: %s", strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Color: false}), ", "))
	}

	var jsResult string
	if len(result.OutputFiles) > 0 {
		jsResult = string(result.OutputFiles[0].Contents)

		targetDir := filepath.Join(a.config.OutputDirectory, base)
		// TODO handle suffix correctly.
		outputFile := filepath.Join(targetDir, strings.TrimSuffix(fileName, ".js")+".js")
		err = os.WriteFile(outputFile, []byte(jsResult), 0644)
		if err != nil {
			log.Fatalf("Failed to write js file: %s", err)
			return err
		}

		log.Debugf("Processed js: %s -> %s\n", outputFile, outputFile)

	} else {
		return errors.New("[esbuild] No output files")
	}

	return nil
}
