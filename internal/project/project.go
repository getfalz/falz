package project

import (
	"craeft/internal/content"
	"craeft/internal/layout"
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
	caches    = &Caches{}
)

type Project struct {
	Server *Server
	Config *CraeftConfig
	assets *Assets
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
		Config: config,
		assets: &Assets{config: config},
	}

	p.Server = NewServer(p)

	return p, nil
}

// TODO skip js/css files too?
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

	var outputFile string
	log.Infof("Matching %s with %s", contentPattern, fileName)
	match, err := zglob.Match(contentPattern, fileName)
	if err != nil {
		log.Errorf("Error matching file: %v\n", err)
		return "", err
	}
	if match {
		var page *content.Content
		if cached, ok := caches.contents.Load(path); ok {
			page = cached.(*content.Content)
		} else {
			page = content.New(path, "md")
		}

		results, err := page.Build(layoutTpl)
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

func (p *Project) Build(isDev bool) {
	p.walk(func(path string, d fs.DirEntry) error {
		if !d.IsDir() {
			_, err := p.BuildPage(path)
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
		log.Debugf("Walking one %s", rel)

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

func (p Project) Close() {
	// TODO
	// * close server
	// * clear caches
	// * close sass server
	p.assets.Close()
}
