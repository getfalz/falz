package project

import (
	"craeft/internal/content"
	"craeft/internal/layout"
	"craeft/internal/utils"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bep/godartsass/v2"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/mattn/go-zglob"
	log "github.com/sirupsen/logrus"
)

var (
	blackList = []string{"config.yaml", "**/layout.xml"}
	caches    = &Caches{}
)

type Project struct {
	Server *Server
	Config *CraeftConfig
}

type Caches struct {
	layouts sync.Map
}

func New(base string) (*Project, error) {
	configPath := filepath.Join(base, "config.yaml")

	log.Info("Loading config from ", configPath)

	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	blackList = append(blackList, filepath.Base(config.OutputDirectory))
	config.Ignore = append(config.Ignore, blackList...)

	p := &Project{
		Config: config,
	}

	p.Server = NewServer(p)

	return p, nil
}

// TODO check if d.Name is any of the fragments
func (p *Project) ignoreFile(path string) bool {
	for _, pat := range p.Config.Ignore {
		match, err := zglob.Match(pat, path)
		if err != nil {
			log.Error(err)
			return false
		}
		if match {
			return true
		}
	}
	return false
}

func (p *Project) GetLayout(path string) (*layout.Layout, error) {
	if cached, ok := caches.layouts.Load(path); ok {
		return cached.(*layout.Layout), nil
	}

	layoutPath, err := utils.FindFile(p.Config.SourceDirectory, path, "layout.xml")
	if err != nil {
		return nil, err
	}

	layoutFile, err := os.ReadFile(layoutPath)
	if err != nil {
		return nil, err
	}

	// layout should know where is the project base folder, for later fragment lookup
	lyt, err := layout.Parse(p.Config.ThemePath, filepath.Dir(layoutPath), layoutFile)
	if err != nil {
		return nil, err
	}

	caches.layouts.Store(path, lyt)
	return lyt, nil
}

// BuildPage builds a single page from a given path
// The path should be relative to the project source
func (p *Project) BuildPage(path string) (string, error) {
	base := filepath.Dir(path)
	fileName := filepath.Base(path)

	lyt, err := p.GetLayout(base)
	if err != nil {
		return "", err
	}
	layoutTpl, err := lyt.Render(base)
	if err != nil {
		return "", err
	}
	contentPattern := lyt.ContentPattern()

	fragments := lyt.Fragments()

	// skip fragment files
	// TODO skip js/css files too?
	for _, f := range fragments {
		match, err := zglob.Match(f, fileName)
		if err != nil {
			return "", err
		}
		if match {
			log.Infof("Skipping file %s", fileName)
			return "", nil
		}
	}

	var outputFile string
	log.Infof("Matching %s with %s", contentPattern, fileName)
	match, err := zglob.Match(contentPattern, fileName)
	if err != nil {
		log.Errorf("Error matching file: %v\n", err)
		return "", err
	}
	if match {
		targetDir := filepath.Join(p.Config.OutputDirectory, base)

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			log.Fatalf("Failed to create output folder: %s", err)
		}

		log.Debugf("-> Building page: %s\n", path)
		finalResult, err := content.BuildPage(layoutTpl, filepath.Join(p.Config.SourceDirectory, path))
		if err != nil {
			log.Fatalf("Failed to write output file: %s", err)
			return "", err
		}

		// TODO use the defined suffix for the corresponding plugin
		outputFile = filepath.Join(targetDir, strings.TrimSuffix(fileName, ".md")+".html")
		err = os.WriteFile(outputFile, []byte(finalResult), 0644)
		if err != nil {
			log.Fatalf("Failed to write output file: %s", err)
			return "", err
		}

		log.Infof("Processed file: %s -> %s\n", path, outputFile)
	}

	return outputFile, nil
}

func (p *Project) Build(isDev bool) {
	err := filepath.WalkDir(p.Config.SourceDirectory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(p.Config.SourceDirectory, path)
		if err != nil {
			return err
		}
		log.Debugf("Visiting %s", rel)

		if p.ignoreFile(rel) {
			log.Infof("Skipping folder %s", rel)
			if d.IsDir() {
				return fs.SkipDir
			} else {
				return nil
			}
		}

		if !d.IsDir() {
			_, err := p.BuildPage(rel)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		log.Error(err)
	}
}

func (p *Project) BuildAssets() {
	err := filepath.WalkDir(p.Config.SourceDirectory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			rel, err := filepath.Rel(p.Config.SourceDirectory, path)
			if err != nil {
				return err
			}
			log.Infof("Assets Visiting %s - %s", path, rel)

			if p.ignoreFile(rel) {
				log.Infof("Skipping file %s", rel)
				return fs.SkipDir
			}

			err = p.BuildAsset(rel)
			if err != nil {
				return err
			}

		}

		return nil
	})

	if err != nil {
		log.Error(err)
	}
}

// TODO rewrite BuildAsset to use esbuild ctx.Rebuild. for sass handling, write a esbuild plugin which parses sass.
func (p *Project) BuildAsset(path string) error {
	lyt, err := p.GetLayout(path)
	if err != nil {
		return err
	}

	if path == lyt.Path() {
		if lyt.CSS != "" {
			// check if file exists
			cssPath := filepath.Join(p.Config.ThemePath, lyt.Path(), lyt.CSS)
			if _, err := os.Stat(cssPath); os.IsNotExist(err) {
				log.Errorf("CSS file not found: %s", cssPath)
			} else {
				var cssResult string
				log.Infof("Processing css: %s", cssPath)
				styles, err := os.ReadFile(cssPath)
				if err != nil {
					return err
				}
				log.Infof("got styles: %s", styles)

				sassPath, err := exec.LookPath(p.Config.StylesConfig.SassPath)
				// parse sass
				if err == nil {
					// TODO keep t in struct
					// build sass
					t, err := godartsass.Start(godartsass.Options{
						DartSassEmbeddedFilename: sassPath,
					})
					if err != nil {
						log.Errorf("Failed to start sass compiler: %v", err)
					}

					defer t.Close()

					var outputStyle godartsass.OutputStyle
					if p.Config.StylesConfig.Compressed {
						outputStyle = godartsass.OutputStyleCompressed
					} else {
						outputStyle = godartsass.OutputStyleExpanded
					}

					var cssSyntax godartsass.SourceSyntax
					switch p.Config.StylesConfig.Parser {
					case "scss":
						cssSyntax = godartsass.SourceSyntaxCSS
					case "indented":
						cssSyntax = godartsass.SourceSyntaxSASS
					default:
						cssSyntax = godartsass.SourceSyntaxSCSS
					}

					// TODO test if imported files can be outside of the project. if so,
					// make a custom importer (interface ImportResolver)
					result, err := t.Execute(godartsass.Args{
						Source:                  string(styles),
						EnableSourceMap:         p.Config.StylesConfig.SourceMap,
						SourceMapIncludeSources: true,
						OutputStyle:             outputStyle,
						SourceSyntax:            cssSyntax,
						IncludePaths:            []string{p.Config.ThemePath},
					})
					if err != nil {
						log.Infof("Failed to compile sass: %v", err)
						return err
					}
					cssResult = result.CSS
				} else {
					// sass bin not found
					log.Infof("Sass binary not found, using plain css: %v", err)
					if errors.Is(err, exec.ErrNotFound) {

						// TODO test if works
						log.Infof("[SASS] Using esbuild css loader.")
						config := api.TransformOptions{
							Loader:     api.LoaderCSS,
							Sourcefile: lyt.CSS,
						}
						if p.Config.StylesConfig.SourceMap {
							config.Sourcemap = api.SourceMapInline
						}
						if p.Config.StylesConfig.Compressed {
							config.MinifyWhitespace = true
							config.MinifyIdentifiers = true
							config.MinifySyntax = true
						}
						result := api.Transform(string(styles), config)
						cssResult = string(result.Code)
					} else {
						return err
					}
				}
				// write css file
				targetDir := filepath.Join(p.Config.OutputDirectory, path)
				log.Infof("Writing css file to %s", targetDir)
				// TODO handle suffix correctly.
				outputFile := filepath.Join(targetDir, strings.TrimSuffix(lyt.CSS, ".scss")+".css")
				log.Infof("Writing css file to %s", outputFile)
				err = os.WriteFile(outputFile, []byte(cssResult), 0644)
				if err != nil {
					log.Fatalf("Failed to write css file: %s", err)
					return err
				}

				log.Infof("Processed css: %s -> %s\n", path, outputFile)
			}

		}

		if lyt.JS != "" {
			// TODO same logic as with css. refactor this
			jsPath := filepath.Join(p.Config.ThemePath, lyt.Path(), lyt.JS)
			if _, err := os.Stat(jsPath); os.IsNotExist(err) {
				log.Errorf("JS file not found: %s", jsPath)
			} else {
				var jsResult string
				log.Infof("Processing js: %s", jsPath)
				scripts, err := os.ReadFile(jsPath)
				if err != nil {
					return err
				}
				log.Infof("got script: %s", scripts)

				config := api.BuildOptions{
					Bundle: true,
					Write:  false,
					// Format:   api.FormatESModule,
					Platform: api.PlatformBrowser,
					Stdin: &api.StdinOptions{
						Contents:   string(scripts),
						ResolveDir: p.Config.ThemePath,
						Sourcefile: lyt.JS,
					},
				}
				if p.Config.ScriptsConfig.SourceMap {
					config.Sourcemap = api.SourceMapInline
				}
				if p.Config.ScriptsConfig.Compressed {
					config.MinifyWhitespace = true
					config.MinifyIdentifiers = true
					config.MinifySyntax = true
				}
				result := api.Build(config)

				if len(result.Errors) > 0 {
					return fmt.Errorf("[esbuid] Found errors: %s", strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Color: false}), ", "))
				}

				if len(result.OutputFiles) > 0 {
					for _, out := range result.OutputFiles {
						fmt.Printf("\npath: %v\ncontents: \n%s\n\n hash: %s\n", out.Path, string(out.Contents), out.Hash)
					}

					jsResult = string(result.OutputFiles[0].Contents)
					log.Infof("got js result: %s", jsResult)

					// TODO same logic as with css. refactor this
					targetDir := filepath.Join(p.Config.OutputDirectory, path)
					log.Infof("Writing js file to %s", targetDir)
					// TODO handle suffix correctly.
					outputFile := filepath.Join(targetDir, strings.TrimSuffix(lyt.JS, ".js")+".js")
					log.Infof("Writing js file to %s", outputFile)
					err = os.WriteFile(outputFile, []byte(jsResult), 0644)
					if err != nil {
						log.Fatalf("Failed to write js file: %s", err)
						return err
					}

					log.Infof("Processed js: %s -> %s\n", path, outputFile)

				} else {
					return errors.New("[esbuild] No output files")
				}
			}

		}
	}

	return nil
}

func (p Project) Close() {
	// TODO
	// * close server
	// * clear caches
	// * close sass server
}
