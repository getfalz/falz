package content

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	log "github.com/sirupsen/logrus"
)

type Content struct {
	path     string
	pipeline string
	output   []ContentResult
}

type ContentResult struct {
	Filename string
	Result   string
}

func New(path, pipeline string) *Content {
	return &Content{
		path:     path,
		pipeline: pipeline,
	}
}

// TODO this method will in future write not only the contents to the template, but all fields the template has
// eg. title, contentN, ...
func (c Content) Build(layoutTpl string) ([]ContentResult, error) {
	if len(c.output) > 0 {
		return c.output, nil
	}

	file, err := os.ReadFile(c.path)
	if err != nil {
		log.Fatalf("Failed to read content file: %s", err)
	}

	content := mdToHTML(file)

	tmpl, err := template.New("contentTemplate").Parse(layoutTpl)
	if err != nil {
		return []ContentResult{}, fmt.Errorf("failed to parse template: %v", err)
	}

	var result strings.Builder
	// TODO add content variables
	data := struct {
		Content string
	}{
		Content: string(content),
	}

	err = tmpl.Execute(&result, data)
	if err != nil {
		return []ContentResult{}, fmt.Errorf("failed to apply content to template: %v", err)
	}

	outputFile := strings.TrimSuffix(c.path, ".md") + ".html"
	res := []ContentResult{
		{Filename: outputFile, Result: result.String()},
	}
	return res, nil
}

func mdToHTML(md []byte) []byte {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}
