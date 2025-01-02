package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"craeft/internal/content"
	"craeft/internal/layout"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/mattn/go-zglob"
	log "github.com/sirupsen/logrus"
)

var (
	blackList = []string{"config.yaml", "**/layout.xml"}
	// TODO move it to Project
	caches = &Caches{}
)

type Project struct {
	Server *Server
	Config *CraeftConfig
	assets *Assets

	pipelines *pipeline.Registry
}

type Caches struct {
	layouts  sync.Map
	contents sync.Map
	assets   sync.Map
}

func New(base string) (*Project, error) {
	configPath := filepath.Join(base, "config.yaml")

	log.Info("Loading config from ", configPath)

	config, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	blackList = append(blackList, filepath.Base(config.OutputDirectory))
	config.Ignore = append(config.Ignore, blackList...)

	rconfig := &pipeline.RegistryConfig{
		Pipelines:       config.Pipelines,
		PipelinesPath:   config.PipelinesPath,
		SourceDirectory: config.SourceDirectory,
	}
	p := &Project{
		Config:    config,
		assets:    &Assets{config: config},
		pipelines: pipeline.NewRegistry(rconfig),
	}

	p.Server = NewServer(p)

	if err := p.pipelines.LoadPipelines(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Project) Build(isDev bool) error {
	return p.walk(func(path string, d fs.DirEntry) error {
		log.Infof("Processing %s", path)
		if d.IsDir() {
			// currentDir := filepath.Dir(path)
			currentDir := path
			log.Infof("is dir: %s", currentDir)

			layouts, err := p.getLayouts(currentDir)
			log.Infof("== layouts: %+v", layouts)
			if err != nil {
				log.Infof("== got layout error: %s", err)
				return err
			}

			for _, lyt := range layouts {
				contentConf := lyt.Content()

				layoutTpl, err := lyt.Render(currentDir)
				if err != nil {
					log.Infof("== got layout render err: %s", err)
					return err
				}

				cpath := filepath.Join(p.Config.SourceDirectory, currentDir, contentConf.Path)
				log.Infof("== getting content: %s, in path %s, totaL: %s", contentConf.Path, path, cpath)
				contents, err := zglob.Glob(cpath)
				log.Infof("== found content: %v with err %s", contents, err)
				if err != nil {
					log.Infof("== got glob err: %s", err)
					return err
				}

				// Index
				log.Info("++ Index")
				for _, contentPath := range contents {
					page, err := p.getContent(contentPath, contentConf.Pipeline)
					if err != nil {
						log.Infof("== got content err: %s", err)
						return err
					}
					page.LoadIndex()
				}

				// Build
				log.Info("++ Building")
				for _, contentPath := range contents {
					page, err := p.getContent(contentPath, contentConf.Pipeline)
					if err != nil {
						log.Infof("== got content err: %s", err)
						return err
					}

					results, err := page.Build(layoutTpl)
					if err != nil {
						log.Infof("== got page build err: %s", err)
						return err
					}

					_, err = p.writeResults(currentDir, results)
					if err != nil {
						log.Infof("== got page write err: %s", err)
						return err
					}
				}
			}
		}

		return nil
	})
}

// BuildPath builds a single page from a given path
// The path should be relative to the project source
func (p *Project) BuildPath(path string) (string, error) {
	base := filepath.Dir(path)
	fileName := filepath.Base(path)

	// TODO: check if the path is withing the project

	layouts, err := p.getLayouts(base)
	if err != nil {
		return "", err
	}

	var outputPath string
	for _, lyt := range layouts {
		if lyt.MatchContent(fileName) {

			contentConf := lyt.Content()

			layoutTpl, err := lyt.Render(base)
			if err != nil {
				return "", err
			}

			// Index
			page, err := p.getContent(path, contentConf.Pipeline)
			if err != nil {
				return "", err
			}

			page.LoadIndex()

			// Build
			results, err := page.Build(layoutTpl)
			if err != nil {
				return "", err
			}

			outputPath, err = p.writeResults(base, results)
			if err != nil {
				return "", err
			}

		}

	}

	// For multiple files, it returns the last one
	return outputPath, nil
}

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
	// fileName := filepath.Base(path)
	// lyt, err := p.GetLayout(path)
	// if err != nil {
	// 	return false
	// }
	//
	// fragments := lyt.Fragments()
	// for _, f := range fragments {
	// 	match, err := zglob.Match(f, fileName)
	// 	if err != nil {
	// 		return false
	// 	}
	// 	if match {
	// 		log.Debugf("Skipping fragment %s", fileName)
	// 		return true
	// 	}
	// }

	return false
}

// GetLayout returns all layouts for a given path
func (p *Project) getLayouts(path string) (map[string]*layout.Layout, error) {
	var results = make(map[string]*layout.Layout)

	log.Infof("getLayouts: im here: theme: %s, path: %s and layout: %s", p.Config.ThemePath, path, p.Config.Layout)
	layoutPaths, err := utils.FindFile(p.Config.ThemePath, path, p.Config.Layout)
	if err != nil {
		return nil, err
	}

	log.Infof("getLayouts: got layouts: %v", layoutPaths)
	for path := range layoutPaths {
		if cached, ok := caches.layouts.Load(path); ok {
			results[path] = cached.(*layout.Layout)
			continue
		}

		layoutFile, err := os.ReadFile(filepath.Join(p.Config.ThemePath, path))
		if err != nil {
			return nil, fmt.Errorf("can't read layout file: %w", err)
		}

		// layout should know where is the project base folder, for later fragment lookup
		layout, err := layout.Parse(p.Config.ThemePath, filepath.Dir(path), layoutFile)
		if err != nil {
			return nil, fmt.Errorf("can't parse layout file: %w", err)
		}

		caches.layouts.Store(path, layout)
		results[path] = layout
	}

	return results, nil
}

func (p Project) writeResults(base string, results []content.ContentResult) (string, error) {
	var outputPath string
	targetDir := filepath.Join(p.Config.OutputDirectory, base)
	log.Infof("targetDir to %s", targetDir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}

	for _, r := range results {
		outputPath = filepath.Join(targetDir, r.Filename)
		log.Infof("outputPath to %s, result path: %s", outputPath, r.Filename)
		err := os.WriteFile(outputPath, []byte(r.Result), 0644)
		if err != nil {
			return "", err
		}
		log.Tracef("Content written to: %s", outputPath)
	}

	return outputPath, nil
}

func (p *Project) getContent(path string, pipeline string) (*content.Content, error) {
	var page *content.Content
	if cached, ok := caches.contents.Load(path); ok {
		page = cached.(*content.Content)
		log.Infof("[del] Got page cache: %+v", page)
	} else {
		pl, err := p.pipelines.Get(pipeline, false)
		if err != nil {
			return nil, err
		}
		page = content.New(path, pl)
		caches.contents.Store(path, page)
	}

	return page, nil
}

// BuildAssets builds all assets from the theme
func (p *Project) BuildAssets() {
	var assets = make(map[string]string)

	p.walk(func(path string, d fs.DirEntry) error {
		// err := filepath.WalkDir(p.Config.ThemePath, func(path string, d fs.DirEntry, err error) error {
		// if err != nil {
		// 	return err
		// }

		if d.IsDir() {
			// rel, err := filepath.Rel(p.Config.ThemePath, path)
			// if err != nil {
			// 	return err
			// }
			log.Infof("Walking through %s", path)
			currentDir := filepath.Dir(path)

			layouts, err := p.getLayouts(currentDir)
			if err != nil {
				return err
			}

			for _, lyt := range layouts {
				for _, asset := range lyt.Assets() {
					npath, err := utils.NormalizePath(lyt.Path(), asset.Source)
					if err != nil {
						return err
					}

					assets[npath] = asset.AssetType

					log.Infof("Adding asset path %s", npath)
				}
			}

		}
		return nil
	})

	// if err != nil {
	// 	log.Error(err)
	// }

	for path, atype := range assets {
		base := filepath.Dir(path)
		asset, err := p.getAsset(path, atype)
		if err != nil {
			log.Error(err)
		}

		results, err := asset.Build()
		if err != nil {
			log.Error(err)
			continue
		}

		_, err = p.writeResults(base, results)
		if err != nil {
			log.Error(err)
		}
	}
}

// TODO rewrite BuildAsset to use esbuild ctx.Rebuild. for sass handling, write a esbuild plugin which parses sass.
func (p *Project) BuildAsset(path string, assetType string) error {
	base := filepath.Dir(path)
	targetDir := filepath.Join(p.Config.OutputDirectory, base)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	asset, err := p.getAsset(path, assetType)
	if err != nil {
		return err
	}
	results, err := asset.Build()
	if err != nil {
		return err
	}

	_, err = p.writeResults(base, results)
	if err != nil {
		return err
	}

	return nil
}

func (p *Project) getAsset(path string, pipeline string) (*content.Asset, error) {
	var asset *content.Asset
	// if cached, ok := caches.contents.Load(path); ok {
	// 	page = cached.(*content.Content)
	// 	log.Infof("[del] Got page cache: %+v", page)
	// } else {
	pl, err := p.pipelines.Get(pipeline, true)
	if err != nil {
		return nil, err
	}
	asset = content.NewAsset(path, pl)
	// caches.contents.Store(path, page)
	// }

	return asset, nil
}

func (p *Project) walk(callback func(path string, d fs.DirEntry) error) error {
	return filepath.WalkDir(p.Config.SourceDirectory, func(path string, d fs.DirEntry, err error) error {
		// if err != nil {
		// 	return err
		// }

		rel, err := filepath.Rel(p.Config.SourceDirectory, path)
		if err != nil {
			return err
		}

		if p.ignoreFile(rel) {
			log.Infof("Skipping folder %s", rel)
			if d.IsDir() {
				return fs.SkipDir
			} else {
				return nil
			}
		}

		if err := callback(rel, d); err != nil {
			return err
		}

		return nil
	})
}

func (p Project) Close() {
	// TODO
	// * close server
	// * clear caches
	// * close sass server
	p.assets.Close()
}
