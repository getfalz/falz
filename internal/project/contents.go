package project

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/content"
	"craeft/internal/layout"

	log "github.com/sirupsen/logrus"
)

var blackList = []string{"config.yaml", "layout.xml"}

func Build(contentFolder, outputFolder string) {
	var lyt *layout.Layout
	var layoutTpl string
	var contentPattern map[string]string
	var lastDir string

	blackList = append(blackList, filepath.Base(outputFolder))
	err := filepath.WalkDir(contentFolder, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		currentFile := d.Name()
		currentDir := filepath.Dir(path)
		if d.IsDir() {
			currentFile = ""
			currentDir = filepath.Join(currentDir, d.Name())
		}

		for _, pat := range blackList {
			currentPath := currentFile
			if d.IsDir() {
				currentPath = currentDir
			}

			match, err := filepath.Match(pat, currentPath)
			if err != nil {
				return err
			}
			if match {
				if d.IsDir() {
					return fs.SkipDir
				} else {
					return nil
				}
			}
		}

		// Loading layout
		// TODO cache layout and content..
		if currentDir != lastDir {
			lastDir = currentDir

			layoutPath, err := FindFile(contentFolder, path, "layout.xml")
			if err != nil {
				return err
			}

			layoutFile, err := os.ReadFile(layoutPath)
			if err != nil {
				log.Fatalf("Failed to read layout file: %s", err)
			}

			lyt, err = layout.Parse(layoutFile)
			if err != nil {
				return err
			}

			layoutTpl, err = lyt.Render(path)
			if err != nil {
				return err
			}
			contentPattern = lyt.GetContents()
		}

		// process contents
		if !d.IsDir() {
			for _, pat := range contentPattern {
				match, err := filepath.Match(pat, currentFile)
				if err != nil {
					log.Errorf("Error matching file: %v\n", err)
					return err
				}
				if match {
					outputCurrent := filepath.Join(outputFolder, currentDir)

					if err := os.MkdirAll(outputCurrent, 0755); err != nil {
						log.Fatalf("Failed to create output folder: %s", err)
					}

					finalResult, err := content.BuildPage(layoutTpl, filepath.Join(contentFolder, path))
					if err != nil {
						log.Fatalf("Failed to write output file: %s", err)
					}

					// TODO use the defined suffix for the corresponding plugin
					outputFile := filepath.Join(outputCurrent, strings.TrimSuffix(currentFile, ".md")+".html")
					err = os.WriteFile(outputFile, []byte(finalResult), 0644)
					if err != nil {
						log.Fatalf("Failed to write output file: %s", err)
					}

					log.Infof("Processed file: %s -> %s\n", path, outputFile)
					break // Only parse the first match
				}
			}
		}

		return nil
	})
	if err != nil {
		log.Error(err)
	}
}
