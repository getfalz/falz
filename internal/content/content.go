package content

import (
	"bytes"
	"os"
	"strings"

	"craeft/internal/pipeline"

	"github.com/adrg/frontmatter"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	log "github.com/sirupsen/logrus"
	"github.com/valyala/fasttemplate"
)

type Content struct {
	path     string
	pipeline *pipeline.Pipeline
	output   []ContentResult
	// TODO make vars all types. see core/params
	vars map[string]string
}

type ContentResult struct {
	Filename string
	Result   string
}

func New(path string, pipeline *pipeline.Pipeline) *Content {
	vars := make(map[string]string)

	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to read content file: %s", err)
	}
	defer file.Close()

	_, err = frontmatter.Parse(file, &vars)
	if err != nil {
		log.Error(err)
	}

	// set some default values
	outputFile := strings.TrimSuffix(path, "."+pipeline.Config().Suffix) + ".html"
	vars["target"] = outputFile

	return &Content{
		path:     path,
		pipeline: pipeline,
		vars:     vars,
	}
}

func (c *Content) LoadIndex() error {
	var vars map[string]string

	// TODO frontmatter is already loaded. Here, we should just strip the frontmatter from the contents, and pass the contents to Index
	file, err := os.Open(c.path)
	if err != nil {
		return err
	}
	defer file.Close()

	content, err := frontmatter.Parse(file, &vars)
	if err != nil {
		return err
	}

	jsVars := c.pipeline.Index(string(content), c.vars)

	c.vars = jsVars

	return nil
}

func varsToInterface(input map[string]string) map[string]interface{} {
	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
func (c Content) Build(layoutTpl string) ([]ContentResult, error) {
	if len(c.output) > 0 {
		return c.output, nil
	}

	file, err := os.ReadFile(c.path)
	if err != nil {
		log.Fatalf("Failed to read content file: %s", err)
	}

	// TODO frontmatter is already loaded. Here, we should just strip the frontmatter from the contents, and pass the contents to Index
	var vars any
	body, err := frontmatter.Parse(bytes.NewReader(file), &vars)
	if err != nil {
		log.Error(err)
	}

	// body = mdToHTML(body)

	transformed := c.pipeline.Transform(string(body), c.vars)

	var results []ContentResult
	t := fasttemplate.New(layoutTpl, "[[", "]]")
	data := varsToInterface(c.vars)
	log.Infof("using data: %v", data)

	for _, r := range transformed {
		data["layout"] = r["result"]
		tplResult := t.ExecuteString(data)

		results = append(results, ContentResult{Filename: r["filename"], Result: tplResult})
	}

	return results, nil

	// tmpl, err := template.New("contentTemplate").Parse(layoutTpl)
	// if err != nil {
	// 	return []ContentResult{}, fmt.Errorf("failed to parse template: %v", err)
	// }
	//
	// var result strings.Builder
	// // TODO add content variables
	// data := struct {
	// 	Content string
	// }{
	// 	Content: string(newContent),
	// }
	//
	// err = tmpl.Execute(&result, data)
	// if err != nil {
	// 	return []ContentResult{}, fmt.Errorf("failed to apply content to template: %v", err)
	// }

	// res := []ContentResult{
	// 	{Filename: outputFile, Result: result.String()},
	// }
	// return res, nil
}

func mdToHTML(md []byte) []byte {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock | parser.Autolink | parser.MathJax
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}
