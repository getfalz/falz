package pipeline

import (
	"context"
	"fmt"
	"regexp"

	"craeft/internal/utils"

	"github.com/gobuffalo/plush"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type NopPipe struct {
}

func (d *NopPipe) Config() PipelineConfig {
	re := regexp.MustCompile(fmt.Sprintf(`\.(%s)\b`, ""))
	return PipelineConfig{
		PipelineName:   "default",
		EntryPoint:     "",
		SourceSuffix:   "",
		SourceSuffixRe: re,
		TargetSuffix:   "",
	}
}
func (d *NopPipe) Index(ctx context.Context, input []byte, vars Vars) error {
	return nil
}
func (d *NopPipe) Transform(ctx context.Context, input []byte, vars Vars) (string, error) {
	return string(input), nil
}

func (d *NopPipe) Hash() string {
	config := d.Config()
	return utils.HashString(config.PipelineName + config.SourceSuffix + config.TargetSuffix)
}

type MdPipe struct {
}

func (m *MdPipe) Config() PipelineConfig {
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
func (m *MdPipe) Index(ctx context.Context, input []byte, vars Vars) error {
	return nil
}
func (m *MdPipe) Transform(ctx context.Context, input []byte, vars Vars) (string, error) {

	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)

	htmlFlags := html.CommonFlags
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	result := markdown.ToHTML(input, p, renderer)

	return string(result), nil
}

func (m *MdPipe) Hash() string {
	config := m.Config()
	return utils.HashString(config.PipelineName + config.SourceSuffix + config.TargetSuffix)
}

type TplPipe struct {
}

func (t TplPipe) Config() PipelineConfig {
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
func (t TplPipe) Index(ctx context.Context, input []byte, vars Vars) error {
	return nil
}
func (t TplPipe) Transform(ctx context.Context, input []byte, vars Vars) (string, error) {
	pctx := plush.NewContextWith(vars)
	result, err := plush.Render(string(input), pctx)
	if err != nil {
		return string(input), fmt.Errorf("tpl pipe: %w", err)
	}

	return result, nil
}

func (t *TplPipe) Hash() string {
	config := t.Config()
	return utils.HashString(config.PipelineName + config.SourceSuffix + config.TargetSuffix)
}
