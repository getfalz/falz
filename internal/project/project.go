package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"craeft/internal/content"
	"craeft/internal/layout"
	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"github.com/mattn/go-zglob"
)

var (
	blackList = []string{"config.yaml", "**/layout.xml"}
	// TODO move it to Project
	caches = &Caches{}
)

type Project struct {
	Server *Server
	Config *CraeftConfig

	pipelines *pipeline.Registry
}

type Caches struct {
	layouts  sync.Map
	contents sync.Map
}

func New(base string) (*Project, error) {
	configPath := filepath.Join(base, "config.yaml")

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
		pipelines: pipeline.NewRegistry(rconfig),
	}

	p.Server = NewServer(p)

	if err := p.pipelines.LoadPipelines(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Project) Build(isDev bool) error {

	log.Print("Building project: %s -> %s\n", p.Config.SourceDirectory, p.Config.OutputDirectory)

	return p.walk(func(path string, d fs.DirEntry) error {
		if d.IsDir() {
			log.Trace(fmt.Sprintf("Building folder %s", path), log.Tag{Key: log.SrcTag, Value: "layout"})
			currentDir := path

			layouts, err := p.getLayouts(currentDir)
			// TODO(err) continue execution instead of returning error
			if err != nil {
				log.Error(err)
				return SkipFile
			}

			for _, lyt := range layouts {
				contentConf := lyt.Content()

				log.Info(fmt.Sprintf("processing layout %s/%s", lyt.Path(), lyt.Name()), log.Tag{Key: log.SrcTag, Value: "layout"})

				layoutTpl, err := lyt.Render(currentDir)
				if err != nil {
					// TODO(err) continue execution instead of returning error
					log.Error(err)
					return SkipFile
				}

				// TODO(content): move this to content struct? see asset. content.Build(), content.Index()
				cpath := filepath.Join(p.Config.SourceDirectory, currentDir, contentConf.Path)
				log.Debug(fmt.Sprintf("- Loading layout contents %s", filepath.Join(currentDir, contentConf.Path)))
				contents, err := zglob.Glob(cpath)
				if err != nil {
					log.Error(err)
					return SkipFile
				}

				var contentBuffer = make(map[string]*content.Content)

				for _, contentPath := range contents {
					path, err := filepath.Rel(p.Config.SourceDirectory, contentPath)
					if err != nil {
						log.Error(err)
						return SkipFile
					}
					page, err := p.getContent(path, contentConf.Pipeline)
					if err != nil {
						log.Error(err)
						return SkipFile
					}
					log.Debug(fmt.Sprintf("Indexing %s", path))

					page.LoadIndex()
					contentBuffer[path] = page
				}
				// }

				for _, contentPath := range contents {
					path, err := filepath.Rel(p.Config.SourceDirectory, contentPath)
					if err != nil {
						log.Error(err)
						return SkipFile
					}
					// page, err := p.getContent(contentPath, contentConf.Pipeline)
					// if err != nil {
					// 	return err
					// }
					page := contentBuffer[path]

					log.Debug(fmt.Sprintf("Building %s", path))
					results, err := page.Build(layoutTpl)
					if err != nil {
						log.Error(err)
						return SkipFile
					}

					_, err = p.writeResults(currentDir, results)
					if err != nil {
						log.Error(err)
						return SkipFile
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

	return false
}

// GetLayout returns all layouts for a given path
func (p *Project) getLayouts(path string) (map[string]*layout.Layout, error) {
	var results = make(map[string]*layout.Layout)

	layoutPaths, err := utils.FindFile(p.Config.ThemePath, path, p.Config.Layout)
	if err != nil {
		return nil, err
	}

	for i := range layoutPaths {
		path := layoutPaths[i]
		if cached, ok := caches.layouts.Load(path); ok {
			results[path] = cached.(*layout.Layout)
			continue
		}

		layoutFile, err := os.ReadFile(filepath.Join(p.Config.ThemePath, path))
		if err != nil {
			return nil, fmt.Errorf("can't read layout file: %w", err)
		}

		// layout should know where is the project base folder, for later fragment lookup
		layout, err := layout.Parse(p.Config.ThemePath, path, layoutFile, p.pipelines)
		if err != nil {
			return nil, fmt.Errorf("can't parse layout file: %w", err)
		}

		log.Trace(fmt.Sprintf("found layout %s", path))
		caches.layouts.Store(path, layout)
		results[path] = layout
	}

	return results, nil
}

func (p Project) writeResults(base string, results []content.ContentResult) (string, error) {
	var outputPath string
	targetDir := filepath.Join(p.Config.OutputDirectory, base)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}

	for _, r := range results {
		outputPath = filepath.Join(targetDir, r.Filename)
		err := os.WriteFile(outputPath, []byte(r.Result), 0644)
		if err != nil {
			return "", err
		}
		log.Trace(fmt.Sprintf("content %s succesfuly written to: %s", r.Filename, outputPath))
	}

	return outputPath, nil
}

func (p *Project) getContent(path string, pipeline string) (*content.Content, error) {
	// var page *content.Content
	// if cached, ok := caches.contents.Load(path); ok {
	// 	page = cached.(*content.Content)
	// 	log.Infof("[del] Got page cache: %+v", page)
	// } else {
	pl, err := p.pipelines.Get(pipeline)
	if err != nil {
		log.Warn(fmt.Sprintf("pipeline '%s' not found for content '%s': %s", pipeline, path, err))
	}
	page, err := content.New(path, pl)
	if err != nil {
		return nil, err
	}
	caches.contents.Store(path, page)
	// }

	return page, nil
}

type assetData struct {
	path     string
	from     string
	pipeline string
}

// BuildAssets builds all assets from the theme
func (p *Project) BuildAssets() {
	var assetslist = make(map[string]assetData)

	log.Print("Building project assets")

	err := filepath.WalkDir(p.Config.ThemePath, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			path, err := filepath.Rel(p.Config.ThemePath, path)
			if err != nil {
				return err
			}
			currentDir := filepath.Dir(path)
			log.Info(fmt.Sprintf("path: %s, currentDir: %s", path, currentDir))

			// TODO(assets): currentDir is always "."?
			layouts, err := p.getLayouts(path)
			if err != nil {
				return err
			}

			for _, lyt := range layouts {
				log.Info(fmt.Sprintf("processing layout %s/%s", lyt.Path(), lyt.Name()), log.Tag{Key: log.SrcTag, Value: "assets"})
				// assets path should be absolute!
				for _, asset := range lyt.Assets() {
					npath := filepath.Join(p.Config.ThemePath, asset.Source)

					log.Debug(fmt.Sprintf("found theme asset %s, total path: %s", asset, npath))
					assetslist[npath] = assetData{pipeline: asset.AssetType, from: "theme", path: npath}
				}

				for _, asset := range lyt.Content().Assets {
					npath := filepath.Join(p.Config.SourceDirectory, asset.Source)

					log.Debug(fmt.Sprintf("found content asset %s, total path: %s", asset, npath))
					assetslist[npath] = assetData{pipeline: asset.AssetType, from: "content", path: npath}
				}
			}

		}
		return nil
	})

	if err != nil {
		log.Error(err)
	}

	log.Info(fmt.Sprintf("Building assets -> %+v", assetslist))

	for path, atype := range assetslist {
		var base string
		if atype.from == "theme" {
			base, err = filepath.Rel(p.Config.ThemePath, filepath.Dir(path))
		} else {
			base, err = filepath.Rel(p.Config.SourceDirectory, filepath.Dir(path))
		}
		if err != nil {
			log.Error(err)
			continue
		}

		asset, err := p.getAsset(atype.path, atype.pipeline)
		if err != nil {
			log.Error(err)
			continue
		}

		log.Debug(fmt.Sprintf("Building asset %s", atype.path))
		results, err := asset.Build()
		if err != nil {
			log.Error(err)
			continue
		}

		_, err = p.writeResults(base, results)
		if err != nil {
			log.Error(err)
			continue
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

// TODO remove err return
func (p *Project) getAsset(path string, pipeline string) (*content.Asset, error) {
	var asset *content.Asset
	// if cached, ok := caches.contents.Load(path); ok {
	// 	page = cached.(*content.Content)
	// 	log.Infof("[del] Got page cache: %+v", page)
	// } else {
	pl, err := p.pipelines.Get(pipeline)
	if err != nil {
		log.Warn(fmt.Sprintf("pipeline %s not found for asset %s: %w", pipeline, path, err))
	}
	asset = content.NewAsset(path, pl)
	// caches.contents.Store(path, page)
	// }

	return asset, nil
}

// walk walks the project source directory and calls the callback for each file
var SkipFile = errors.New("skip this file")

func (p *Project) walk(callback func(path string, d fs.DirEntry) error) error {
	return filepath.WalkDir(p.Config.SourceDirectory, func(path string, d fs.DirEntry, err error) error {

		rel, err := filepath.Rel(p.Config.SourceDirectory, path)
		if err != nil {
			return err
		}

		if p.ignoreFile(rel) {
			if d.IsDir() {
				return fs.SkipDir
			} else {
				return nil
			}
		}

		if err := callback(rel, d); err != nil {
			if err == SkipFile {
				return nil
			}
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
	// p.assets.Close()
}
