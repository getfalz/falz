package project

import (
	"craeft/internal/content"
	"craeft/internal/layout"
	"craeft/internal/pipeline"
	"craeft/internal/utils"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/mattn/go-zglob"
	log "github.com/sirupsen/logrus"
)

var (
	blackList = []string{"config.yaml", "**/layout.xml"}
	// TODO move it to Project
	caches = &Caches{}
)

type Project struct {
	Server    *Server
	Config    *CraeftConfig
	assets    *Assets
	pipelines map[string]*pipeline.Pipeline
}

type Caches struct {
	layouts  sync.Map
	contents sync.Map
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

	p := &Project{
		Config:    config,
		assets:    &Assets{config: config},
		pipelines: make(map[string]*pipeline.Pipeline),
	}

	p.Server = NewServer(p)

	if err := p.loadPipelines(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Project) loadPipelines() error {

	// Check if the pipelines directory exists
	if _, err := os.Stat(p.Config.PipelinesPath); os.IsNotExist(err) {
		return fmt.Errorf("pipelines directory does not exist: %s", p.Config.PipelinesPath)
	}

	for _, pipelineName := range p.Config.Pipelines {
		pipelinePath := filepath.Join(p.Config.PipelinesPath, pipelineName)
		log.Infof("Loading pipeline %s from %s", pipelineName, pipelinePath)

		if info, err := os.Stat(pipelinePath); os.IsNotExist(err) || !info.IsDir() {
			return fmt.Errorf("pipeline directory does not exist or is not a directory: %s", pipelinePath)
		}

		pipeline, err := pipeline.New(pipelinePath)
		if err != nil {
			return fmt.Errorf("error loading pipeline %s: %v", pipelineName, err)
		}

		log.Infof("Loaded pipeline %s from %v", pipelineName, pipeline)
		p.pipelines[pipelineName] = pipeline
	}

	return nil
}

// TODO skip js/css files too?
// TODO move ignoreFiles to tools, so that server is not dependent of Project?
func (p *Project) ignoreFile(path string) bool {
	fileName := filepath.Base(path)
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
	lyt, err := p.GetLayout(path)
	if err != nil {
		return false
	}

	fragments := lyt.Fragments()
	for _, f := range fragments {
		match, err := zglob.Match(f, fileName)
		if err != nil {
			return false
		}
		if match {
			log.Debugf("Skipping fragment %s", fileName)
			return true
		}
	}

	return false
}

func (p *Project) GetLayout(path string) (*layout.Layout, error) {
	if cached, ok := caches.layouts.Load(path); ok {
		return cached.(*layout.Layout), nil
	}

	layoutPath, err := utils.FindFile(p.Config.ThemePath, path, "layout.xml")
	if err != nil {
		return nil, err
	}

	layoutFile, err := os.ReadFile(layoutPath)
	if err != nil {
		return nil, err
	}

	// layout should know where is the project base folder, for later fragment lookup
	layoutBase, err := filepath.Rel(p.Config.ThemePath, layoutPath)
	if err != nil {
		return nil, err
	}
	lyt, err := layout.Parse(p.Config.ThemePath, filepath.Dir(layoutBase), layoutFile)
	if err != nil {
		return nil, err
	}

	caches.layouts.Store(path, lyt)
	return lyt, nil
}

// UpdatePath builds a single page from a given path
// The path should be relative to the project source
func (p *Project) UpdatePath(path string) (string, error) {
	base := filepath.Dir(path)
	fileName := filepath.Base(path)

	lyt, err := p.GetLayout(base)
	if err != nil {
		return "", err
	}

	var outputFile string
	if lyt.HasContent(fileName) {
		results, err := p.buildPageWithLayout(lyt, path)
		if err != nil {
			return "", err
		}

		log.Debugf("-> Building page: %s\n", path)
		targetDir := filepath.Join(p.Config.OutputDirectory, base)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return "", err
		}

		for _, r := range results {
			outputFile = filepath.Join(p.Config.OutputDirectory, r.Filename)
			err = os.WriteFile(outputFile, []byte(r.Result), 0644)
			if err != nil {
				return "", err
			}
			log.Infof("Processed file: %s -> %s\n", path, outputFile)
		}
	}

	// for multiple files, it returns the last one
	return outputFile, nil
}

func (p *Project) buildPageWithLayout(lyt *layout.Layout, path string) ([]content.ContentResult, error) {
	var page *content.Content

	if cached, ok := caches.contents.Load(path); ok {
		page = cached.(*content.Content)
		log.Infof("Got page cache: %+v", page)
	} else {
		ctn := lyt.Content()
		pl, err := p.GetPipeline(ctn.Pipeline)
		if err != nil {
			return nil, err
		}
		if err := pl.Load(); err != nil {
			return nil, err
		}
		page = content.New(path, pl)
	}

	base := filepath.Dir(path)
	layoutTpl, err := lyt.Render(base)
	if err != nil {
		return nil, err
	}

	results, err := page.Build(layoutTpl)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (p *Project) indexPageWithLayout(lyt *layout.Layout, path string) (map[string]string, error) {
	var page *content.Content
	if cached, ok := caches.contents.Load(path); ok {
		page = cached.(*content.Content)
		log.Infof("Got page cache: %+v", page)
	} else {
		ctn := lyt.Content()
		pl, err := p.GetPipeline(ctn.Pipeline)
		if err != nil {
			return nil, err
		}
		if err := pl.Load(); err != nil {
			return nil, err
		}
		page = content.New(path, pl)
	}

	return nil, nil
}

func (p *Project) Build(isDev bool) {
	// index
	// p.walk(func(path string, d fs.DirEntry) error {
	// 	if !d.IsDir() {
	// 		_, err := p.BuildPage(path)
	// 		if err != nil {
	// 			return err
	// 		}
	// 	}
	//
	// 	return nil
	// })

	p.walk(func(path string, d fs.DirEntry) error {
		if !d.IsDir() {
			_, err := p.UpdatePath(path)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (p *Project) BuildAssets() {
	p.walk(func(path string, d fs.DirEntry) error {
		if d.IsDir() {
			log.Infof("Assets Visiting %s", path)

			err := p.BuildAsset(path)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *Project) walk(callback func(path string, d fs.DirEntry) error) {
	err := filepath.WalkDir(p.Config.SourceDirectory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

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

	base := filepath.Dir(lyt.CSS)
	targetDir := filepath.Join(p.Config.OutputDirectory, base)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	var errors error
	if path == lyt.Path() {
		if lyt.CSS != "" {
			if err := p.assets.BuildCSS(lyt.Path(), lyt.CSS); err != nil {
				errors = fmt.Errorf("error building css: %s", err)
			}
		}

		if lyt.JS != "" {
			if err := p.assets.BuildJS(lyt.Path(), lyt.JS); err != nil {
				errors = fmt.Errorf("%s, error building js: %s", errors, err)
			}
		}
	}

	return errors
}

func (p *Project) GetPipeline(name string) (*pipeline.Pipeline, error) {
	var m pipeline.Pipeline
	if m, ok := p.pipelines[name]; ok {
		return m, nil
	}
	return &m, fmt.Errorf("Can't find pipeline %s", name)
}

func (p Project) Close() {
	// TODO
	// * close server
	// * clear caches
	// * close sass server
	p.assets.Close()
}
