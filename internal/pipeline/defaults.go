package pipeline

import (
	"context"
	"fmt"
	"regexp"

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
func (d *NopPipeline) Index(ctx context.Context, input []byte, vars Vars) error {
	return nil
}
func (d *NopPipeline) Transform(ctx context.Context, input []byte, vars Vars) ([]map[string]string, error) {
	results := []map[string]string{
		{
			"filename": vars["target"].(string),
			"result":   string(input),
		},
	}
	return results, nil
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
func (m *MdPipeline) Index(ctx context.Context, input []byte, vars Vars) error {
	return nil
}
func (m *MdPipeline) Transform(ctx context.Context, input []byte, vars Vars) ([]map[string]string, error) {

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
	return results, nil
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
func (t TplPipeline) Index(ctx context.Context, input []byte, vars Vars) error {
	return nil
}
func (t TplPipeline) Transform(ctx context.Context, input []byte, vars Vars) ([]map[string]string, error) {
	pctx := plush.NewContextWith(vars)
	result, err := plush.Render(string(input), pctx)
	if err != nil {
		return []map[string]string{
			{
				"filename": vars["target"].(string),
				"result":   string(input),
			},
		}, fmt.Errorf("error transforming template %s: %s", vars["target"], err)
	}

	results := []map[string]string{
		{
			"filename": vars["target"].(string),
			"result":   result,
		},
	}
	return results, nil
}
