package content

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/pipeline"

	"github.com/adrg/frontmatter"
	log "github.com/sirupsen/logrus"
	"github.com/valyala/fasttemplate"
)

type Content struct {
	path     string
	pipeline pipeline.PipelineScript
	output   []ContentResult
	// TODO make vars all types. see core/params
	vars map[string]string
}

type ContentResult struct {
	Filename string
	Result   string // TODO: make it []byte
}

// New creates a new content object with an absolute path and a pipeline
func New(path string, pipeline pipeline.PipelineScript) *Content {
	vars := make(map[string]string)

	// set some default values
	fileName := filepath.Base(path)
	outputFile := strings.TrimSuffix(fileName, pipeline.Config().SourceSuffix) + pipeline.Config().TargetSuffix
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
}
