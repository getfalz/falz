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
func (d *NopPipeline) Transform(ctx context.Context, input []byte, vars Vars) (string, error) {
	return string(input), nil
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
func (m *MdPipeline) Transform(ctx context.Context, input []byte, vars Vars) (string, error) {

	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)

	htmlFlags := html.CommonFlags
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	result := markdown.ToHTML(input, p, renderer)

	return string(result), nil
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
func (t TplPipeline) Transform(ctx context.Context, input []byte, vars Vars) (string, error) {
	pctx := plush.NewContextWith(vars)
	result, err := plush.Render(string(input), pctx)
	if err != nil {
		return string(input), fmt.Errorf("tpl pipe: %w", err)
	}

	return result, nil
}
