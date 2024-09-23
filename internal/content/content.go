package content

import (
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// TODO this method will in future write not only the contents to the template, but all fields the template has
// eg. title, contentN, ...
func BuildPage(layoutTpl, path string) (string, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read content file: %s", err)
	}

	content := mdToHTML(file)

	tmpl, err := template.New("contentTemplate").Parse(layoutTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	var result strings.Builder
	// Data to be injected into the template, for example:
	data := struct {
		Content string
	}{
		Content: string(content),
	}

	// Execute the template and replace the {{.Content}} placeholder
	err = tmpl.Execute(&result, data)
	if err != nil {
		return "", fmt.Errorf("failed to apply content to template: %v", err)
	}

	return result.String(), nil
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
