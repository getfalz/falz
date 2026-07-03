package pipeline

import (
	"context"
	"fmt"

	"craeft/internal/utils"

	"github.com/gobuffalo/plush"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type NopPipe struct {
}

func (d *NopPipe) Config() PipelineConfig {
	return PipelineConfig{
		PipelineName: "default",
		EntryPoint:   "",
	}
}
func (d *NopPipe) Index(ctx context.Context, input []byte, vars Vars) error {
	return nil
}
func (d *NopPipe) Transform(ctx context.Context, input []byte, vars Vars) (TransformResult, error) {
	return TransformResult{Content: input}, nil
}

func (d *NopPipe) Hash() string {
	config := d.Config()
	return utils.HashString(config.PipelineName)
}

type MdPipe struct {
}

func (m *MdPipe) Config() PipelineConfig {
	return PipelineConfig{
		PipelineName: "md",
		EntryPoint:   "",
	}
}
func (m *MdPipe) Index(ctx context.Context, input []byte, vars Vars) error {
	return nil
}
func (m *MdPipe) Transform(ctx context.Context, input []byte, vars Vars) (TransformResult, error) {

	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)

	htmlFlags := html.CommonFlags
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	result := markdown.ToHTML(input, p, renderer)

	return TransformResult{Content: result}, nil
}

func (m *MdPipe) Hash() string {
	config := m.Config()
	return utils.HashString(config.PipelineName)
}

type TplPipe struct {
}

func (t TplPipe) Config() PipelineConfig {
	return PipelineConfig{
		PipelineName: "default",
		EntryPoint:   "",
	}
}
func (t TplPipe) Index(ctx context.Context, input []byte, vars Vars) error {
	return nil
}
func (t TplPipe) Transform(ctx context.Context, input []byte, vars Vars) (TransformResult, error) {
	pctx := plush.NewContextWith(vars)
	result, err := plush.Render(string(input), pctx)
	if err != nil {
		return TransformResult{Content: input}, fmt.Errorf("tpl pipe: %w", err)
	}

	return TransformResult{Content: []byte(result)}, nil
}

func (t *TplPipe) Hash() string {
	config := t.Config()
	return utils.HashString(config.PipelineName)
}
