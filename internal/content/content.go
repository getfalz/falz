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
	name     string
	path     string
	pipeline pipeline.PipelineScript
	vars     pipeline.Vars
}

type ContentResult struct {
	Filename string
	Result   string // TODO: make it []byte
}

// New creates a new content object with an absolute path and a pipeline
func New(name, path string, pl pipeline.PipelineScript, variables any) (*Content, error) {

	fileName := filepath.Base(path)
	log.Trace(fmt.Sprintf("New content %s with pipeline %v", fileName, pl))
	target := pl.Config().SourceSuffixRe.ReplaceAllString(fileName, "."+pl.Config().TargetSuffix)

	vars := pipeline.NewVars(target)
	vars.Set("vars", variables)
	return &Content{
		name:     name,
		path:     path,
		pipeline: pl,
		vars:     vars,
	}, nil
}

// @ts type global.iCtx
// @doc Context passed into the `index` function.
// @field vars: Record<string, unknown> - Variables set in the config.yaml file. Custom key/value mappings loaded from `config.yaml`. Values may include environment‑variable references (e.g. `$HOME/docs`), which will be expanded at runtime.
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

	jsVars := c.pipeline.Index(content, c.vars)

	c.vars.Set("index", jsVars)

	return nil
}

// @ts type global.tCtx
// @doc Context passed into the `transform` function.
// @pred iCtx &
// @field index: Record<string, unknown>
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
	t := fasttemplate.New(layoutTpl, "{{", "}}")

	for _, r := range transformed {
		if r["filename"] != "" {
			c.vars.Set(c.name, r["result"])
			tplResult := t.ExecuteString(c.vars)

			results = append(results, ContentResult{Filename: r["filename"], Result: tplResult})
		}
	}

	return results, nil
}
