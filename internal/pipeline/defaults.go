package pipeline

import (
	"fmt"
	"regexp"

	"craeft/internal/log"

	"github.com/gobuffalo/plush"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type NopPipeline struct {
}

func (d *NopPipeline) Config() PipelineConfig {
	re := regexp.MustCompile(fmt.Sprintf(`\.(%s)\b`, ""))
	return PipelineConfig{
		PipelineName:   "default",
		EntryPoint:     "",
		SourceSuffix:   "",
		SourceSuffixRe: re,
		TargetSuffix:   "",
	}
}
func (d *NopPipeline) Index(input []byte, vars Vars) Vars {
	return vars
}
func (d *NopPipeline) Transform(input []byte, vars Vars) []map[string]string {
	results := []map[string]string{
		{
			"filename": vars["target"].(string),
			"result":   string(input),
		},
	}
	return results
}

type MdPipeline struct {
}

func (m *MdPipeline) Config() PipelineConfig {
	pattern := "md|markdown|mkd|mkdn|mdown"

	re := regexp.MustCompile(fmt.Sprintf(`\.(%s)\b`, pattern))
	return PipelineConfig{
		PipelineName:   "md",
		EntryPoint:     "",
		SourceSuffix:   pattern,
		TargetSuffix:   "html",
		SourceSuffixRe: re,
	}
}
func (m *MdPipeline) Index(input []byte, vars Vars) Vars {
	return vars
}
func (m *MdPipeline) Transform(input []byte, vars Vars) []map[string]string {

	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)

	htmlFlags := html.CommonFlags
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	result := markdown.ToHTML(input, p, renderer)

	results := []map[string]string{
		{
			"filename": vars["target"].(string),
			"result":   string(result),
		},
	}
	return results
}

type TplPipeline struct {
}

func (t TplPipeline) Config() PipelineConfig {
	pattern := "tpl|tmpl"

	re := regexp.MustCompile(fmt.Sprintf(`\.(%s)\b`, pattern))

	return PipelineConfig{
		PipelineName:   "default",
		EntryPoint:     "",
		SourceSuffix:   pattern,
		TargetSuffix:   "html",
		SourceSuffixRe: re,
	}
}
func (t TplPipeline) Index(input []byte, vars Vars) Vars {
	return vars
}
func (t TplPipeline) Transform(input []byte, vars Vars) []map[string]string {
	ctx := plush.NewContextWith(vars)
	result, err := plush.Render(string(input), ctx)
	if err != nil {
		log.Error(fmt.Errorf("error transforming template %s: %s", vars["target"], err))
		result = string(input)
	}

	results := []map[string]string{
		{
			"filename": vars["target"].(string),
			"result":   result,
		},
	}
	return results
}
