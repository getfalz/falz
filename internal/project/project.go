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
	// TODO move it to Project
	caches    = &Caches{}
	blackList = []string{fmt.Sprintf("%s/**", projectRootFolder), projectRootFolder}
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

func New(config *CraeftConfig) (*Project, error) {
	if err := config.IsValid(); err != nil {
		return nil, err
	}

	outputBase := filepath.Base(config.OutputPath)
	blackList = append(blackList, fmt.Sprintf("%s/**", outputBase), outputBase)
	config.Ignore = append(config.Ignore, blackList...)

	rconfig := &pipeline.RegistryConfig{
		Pipelines:       config.Pipes,
		PipelinesPath:   config.PipesPath,
		SourceDirectory: config.SourcePath,
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

func (p *Project) Build(useCache bool) error {
	var assetslist = make(map[string]assetData)

	err := p.walk(func(path string, d fs.DirEntry) error {
		if d.IsDir() {
			log.Trace(fmt.Sprintf("Building folder %q", path), log.Tag{Key: log.SrcTag, Value: "layout"})
			currentDir := path

			layouts, err := p.getLayouts(currentDir, useCache)
			if err != nil {
				log.Error(err)
				return SkipFile
			}

			for _, lyt := range layouts {
				layoutTpl, err := lyt.Render(currentDir)
				if err != nil {
					log.Error(err)
					return SkipFile
				}

				for _, contentConf := range lyt.Contents() {

					log.Info(fmt.Sprintf("processing layout '%s/%s'", lyt.Path(), lyt.Name()), log.Tag{Key: log.SrcTag, Value: "layout"})

					// TODO(content): move this to content struct? see asset. content.Build(), content.Index()
					// TODO(important): path should not point out of the project
					cpath := filepath.Join(p.Config.SourcePath, currentDir, contentConf.Path)
					log.Debug(fmt.Sprintf("- Looking for layout content path %q", filepath.Join(currentDir, contentConf.Path)))
					contents, err := zglob.Glob(cpath)
					if err != nil {
						if errors.Is(err, os.ErrNotExist) {
							log.Warn(fmt.Sprintf("No content %s found for layout %s", filepath.Join(currentDir, contentConf.Path), lyt.Name()))
							return SkipFile
						}
						log.Error(err)
						return SkipFile
					}

					var contentBuffer = make(map[string]*content.Content)

					if len(contents) == 0 {
						log.Warn(fmt.Sprintf("layout %s/%s content path %q has no matches", lyt.Path(), lyt.Name(), contentConf.Path))
					}
					for _, contentPath := range contents {
						path, err := filepath.Rel(p.Config.SourcePath, contentPath)
						if err != nil {
							log.Error(err)
							return SkipFile
						}
						page, err := p.getContent(contentConf.Name, path, contentConf.Pipe)
						if err != nil {
							log.Error(err)
							return SkipFile
						}
						log.Debug(fmt.Sprintf("Indexing %q", path))

						page.LoadIndex()
						contentBuffer[path] = page
					}
					// }

					for _, contentPath := range contents {
						path, err := filepath.Rel(p.Config.SourcePath, contentPath)
						if err != nil {
							log.Error(err)
							return SkipFile
						}
						// page, err := p.getContent(contentPath, contentConf.Pipeline)
						// if err != nil {
						// 	return err
						// }
						page := contentBuffer[path]

						log.Debug(fmt.Sprintf("Building %q", path))
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

					for _, asset := range contentConf.Assets {
						npath := filepath.Join(p.Config.SourcePath, asset.Source)

						log.Debug(fmt.Sprintf("found content asset %q, total path: %q", asset, npath))
						assetslist[npath] = assetData{pipeline: asset.AssetType, from: "content", path: npath}
					}
				}

				// assets path should be absolute!
				for _, asset := range lyt.Assets() {
					npath := filepath.Join(p.Config.SchemePath, asset.Source)

					log.Debug(fmt.Sprintf("found theme asset %q, total path: %q", asset, npath))
					assetslist[npath] = assetData{pipeline: asset.AssetType, from: "theme", path: npath}
				}

			}
		}

		return nil
	})

	log.Info(fmt.Sprintf("Building assets -> %+v", assetslist))

	for path, atype := range assetslist {
		var base string
		// atype.from says where the asset was found. inside a content or the layout
		if atype.from == "theme" {
			base, err = filepath.Rel(p.Config.SchemePath, filepath.Dir(path))
		} else {
			base, err = filepath.Rel(p.Config.SourcePath, filepath.Dir(path))
		}
		if err != nil {
			log.Error(err)
			continue
		}

		matches, err := zglob.Glob(path)
		if err != nil {
			log.Error(fmt.Errorf("failed building asset with path %q: %v", path, err))
			continue
		}

		for _, match := range matches {
			asset, err := p.getAsset(match, atype.pipeline)
			if err != nil {
				log.Error(err)
				continue
			}

			log.Debug(fmt.Sprintf("Building asset %q", match))
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

	return err
}

// BuildPath builds a single page from a given path
// The path should be relative to the project source
func (p *Project) BuildPath(path string) (string, error) {
	base := filepath.Dir(path)
	fileName := filepath.Base(path)

	// TODO: check if the path is withing the project

	layouts, err := p.getLayouts(base, false)
	if err != nil {
		return "", err
	}

	var outputPath string
	for _, lyt := range layouts {
		if lyt.MatchContent(fileName) {
			for _, contentConf := range lyt.Contents() {
				if contentConf.MatchContent(fileName) {

					layoutTpl, err := lyt.Render(base)
					if err != nil {
						return "", err
					}

					// Index
					page, err := p.getContent(contentConf.Name, path, contentConf.Pipe)
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

		}

	}

	// For multiple files, it returns the last one
	return outputPath, nil
}

func (p *Project) ignoreFile(path string) bool {
	path = filepath.Clean(path)
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
func (p *Project) getLayouts(path string, useCache bool) (map[string]*layout.Layout, error) {
	var results = make(map[string]*layout.Layout)

	layoutPaths, err := utils.FindFile(p.Config.SchemePath, path, p.Config.Layout)
	if err != nil {
		return nil, fmt.Errorf("cannot find layout for path %q: %w", path, err)
	}

	for i := range layoutPaths {
		path := layoutPaths[i]
		if cached, ok := caches.layouts.Load(path); ok && useCache {
			results[path] = cached.(*layout.Layout)
			log.Trace(fmt.Sprintf("using cached layout %s", path))
			continue
		}

		layoutPath := filepath.Join(p.Config.SchemePath, path)
		layoutFile, err := os.ReadFile(layoutPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read layout file %q: %w", layoutPath, err)
		}

		// layout should know where is the project base folder, for later fragment lookup
		layout, err := layout.Parse(p.Config.SchemePath, path, layoutFile, p.pipelines, p.Config.FsWatcher.HotReload)
		if err != nil {
			return nil, fmt.Errorf("failed to parse layout file %q: %w", layoutPath, err)
		}

		log.Trace(fmt.Sprintf("using layout %s", layoutPath))
		caches.layouts.Store(path, layout)
		results[path] = layout
	}

	return results, nil
}

func (p Project) writeResults(base string, results []content.ContentResult) (string, error) {
	var outputPath string
	targetDir := filepath.Join(p.Config.OutputPath, base)
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

func (p *Project) getContent(name, path string, pipeline string) (*content.Content, error) {
	// var page *content.Content
	// if cached, ok := caches.contents.Load(path); ok {
	// 	page = cached.(*content.Content)
	// 	log.Infof("[del] Got page cache: %+v", page)
	// } else {
	pl, err := p.pipelines.Get(pipeline)
	if err != nil {
		if pipeline != "" {
			log.Warn(fmt.Sprintf("pipeline '%s' not found for content '%s': %s", pipeline, path, err))
		}
	}
	page, err := content.New(name, path, pl, p.Config.Variables)
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
	return filepath.WalkDir(p.Config.SourcePath, func(path string, d fs.DirEntry, err error) error {

		rel, err := filepath.Rel(p.Config.SourcePath, path)
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
