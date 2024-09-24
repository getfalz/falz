package project

import (
	"craeft/internal/content"
	"craeft/internal/layout"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

var blackList = []string{"config.yaml", "layout.xml"}

type Project struct {
	Server  *Server
	Config  *CraeftConfig
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

func (p *Project) ignoreFile(path string) bool {
	for _, pat := range p.Config.Ignore {
		match, err := filepath.Match(pat, path)
		if err != nil {
			log.Error(err)
			return false
		}
		return match
	}
	return false
}

func (p *Project) GetLayout(path string) (*layout.Layout, error) {
	if cached, ok := p.layouts.Load(path); ok {
		return cached.(*layout.Layout), nil
	}

	layoutPath, err := FindFile(p.Config.SourceDirectory, path, "layout.xml")
	if err != nil {
		return nil, err
	}

	layoutFile, err := os.ReadFile(layoutPath)
	if err != nil {
		return nil, err
	}

	lyt, err := layout.Parse(layoutFile)
	if err != nil {
		return nil, err
	}

	p.layouts.Store(path, lyt)
	return lyt, nil
}

// BuildPage builds a single page from a given path
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
	contentPattern := lyt.GetContents()

	var outputFile string
	for _, pat := range contentPattern {
		match, err := filepath.Match(pat, fileName)
		if err != nil {
			log.Errorf("Error matching file: %v\n", err)
			return "", err
		}
		if match {
			targetDir := filepath.Join(p.Config.OutputDirectory, base)

			if err := os.MkdirAll(targetDir, 0755); err != nil {
				log.Fatalf("Failed to create output folder: %s", err)
			}

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
			break // Only parse the first match
		}
	}

	return outputFile, nil
}

func (p *Project) Build(isDev bool) {
	err := filepath.WalkDir(p.Config.SourceDirectory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if p.ignoreFile(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			} else {
				return nil
			}
		}

		// Process contents
		if !d.IsDir() {
			_, err := p.BuildPage(path)
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
