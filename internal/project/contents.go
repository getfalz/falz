package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/content"
	"craeft/internal/layout"

	log "github.com/sirupsen/logrus"
)

func Build(contentFolder, outputFolder string) {
	err := filepath.WalkDir(contentFolder, func(path string, d fs.DirEntry, err error) error {
		// TODO skip folders/files in black list
		// err := filepath.Walk(contentFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		layoutFile, err := os.ReadFile(filepath.Join(contentFolder, "layout.xml"))
		if err != nil {
			log.Fatalf("Failed to read layout file: %s", err)
		}

		i, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Println(path, i.Size())
		return nil
	})
	if err != nil {
		log.Println(err)
	}
}

func Buildo(contentFolder, outputFolder string) {
	// TODO getLayoutFile method. if the given folder is not the source folder,
	// and no layout.xml file is found, then the parent folder is checked.
	layoutFile, err := os.ReadFile(filepath.Join(contentFolder, "layout.xml"))
	if err != nil {
		log.Fatalf("Failed to read layout file: %s", err)
	}

	if err := os.MkdirAll(outputFolder, 0755); err != nil {
		log.Fatalf("Failed to create output folder: %s", err)
	}

	lyt, err := layout.Parse(layoutFile)
	layoutTpl, err := lyt.Render()

	// TODO read only files matching the content include attribute (*.md)
	files, err := os.ReadDir(contentFolder)
	if err != nil {
		log.Fatalf("Failed to read content folder: %s", err)
	}

	log.Info("Building project...")
	for _, file := range files {
		// TODO this may be not necessary
		if filepath.Ext(file.Name()) == ".md" {
			// TODO after reading the file, pass the contents to the corresponding js plugin

			// Apply the content to the base template
			finalResult, err := content.BuildPage(layoutTpl, filepath.Join(contentFolder, file.Name()))
			if err != nil {
				log.Fatalf("Failed to write output file: %s", err)
			}

			// TODO maybe use the defined suffix for the corresponding plugin
			outputFile := filepath.Join(outputFolder, strings.TrimSuffix(file.Name(), ".md")+".html")
			log.Infof("Writing to file: %s", outputFile)
			err = os.WriteFile(outputFile, []byte(finalResult), 0644)
			if err != nil {
				log.Fatalf("Failed to write output file: %s", err)
			}

			log.Infof("Processed file: %s -> %s\n", file.Name(), outputFile)
		}
	}
}
