package content

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"craeft/internal/pipeline"

	"craeft/internal/log"

	"github.com/adrg/frontmatter"
	"github.com/valyala/fasttemplate"
)

type Content struct {
	path     string
	pipeline pipeline.PipelineScript
	vars     pipeline.Vars
}

type ContentResult struct {
	Filename string
	Result   string // TODO: make it []byte
}

// New creates a new content object with an absolute path and a pipeline
func New(path string, pl pipeline.PipelineScript) (*Content, error) {

	fileName := filepath.Base(path)
	log.Trace(fmt.Sprintf("New content %s with pipeline %v", fileName, pl))
	target := pl.Config().SourceSuffixRe.ReplaceAllString(fileName, "."+pl.Config().TargetSuffix)

	vars := pipeline.NewVars(target)
	return &Content{
		path:     path,
		pipeline: pl,
		vars:     vars,
	}, nil
}

func (c *Content) LoadIndex() error {
	var vars map[string]any

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

	for k, v := range vars {
		c.vars[k] = v
	}
	log.Debug(fmt.Sprintf("using data: %v", c.vars))

	jsVars := c.pipeline.Index(content, c.vars)

	c.vars = jsVars

	return nil
}

func (c Content) Build(layoutTpl string) ([]ContentResult, error) {
	file, err := os.ReadFile(c.path)
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to read content file: %s", err))
	}

	// TODO frontmatter is already loaded. Here, we should just strip the frontmatter from the contents, and pass the contents to Index
	var vars any
	body, err := frontmatter.Parse(bytes.NewReader(file), &vars)
	if err != nil {
		log.Error(err)
	}

	transformed := c.pipeline.Transform(body, c.vars)

	var results []ContentResult
	t := fasttemplate.New(layoutTpl, "[[", "]]")
	log.Info(fmt.Sprintf("using data: %v", c.vars))

	for _, r := range transformed {
		c.vars["layout"] = r["result"]
		tplResult := t.ExecuteString(c.vars)

		results = append(results, ContentResult{Filename: r["filename"], Result: tplResult})
	}

	return results, nil
}
