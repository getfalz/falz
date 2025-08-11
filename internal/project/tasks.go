package project

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/pipeline"
	"fmt"
	"path/filepath"
	"sync/atomic"

	"github.com/valyala/fasttemplate"
)

type IndexTask struct {
	P    *Project
	Page PagePlan
}

// @ts type global.iCtx
// @doc Context passed into the `index` function.
// @field vars: Record<string, unknown> - Variables set in the config.yaml file. Custom key/value mappings loaded from `config.yaml`. Values may include environment‑variable references (e.g. `$HOME/docs`), which will be expanded at runtime.
func (t IndexTask) Key() string { return "index:" + t.Page.RelContent }
func (t IndexTask) Run(ctx context.Context) error {
	log.TTrace(log.Build).
		Str("rel", t.Page.RelContent).
		Str("pipe", t.Page.Pipe).
		Msg("index start")

	pl, err := t.P.pipelines.Get(t.Page.Pipe)
	if err != nil {
		return fmt.Errorf("index pipeline %s: %w", t.Page.RelContent, err)
	}
	data, err := t.P.readSource(t.Page.RelContent)
	if err != nil {
		return fmt.Errorf("index read %s: %w", t.Page.RelContent, err)
	}
	cfg := pl.Config()
	target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(t.Page.RelContent), "."+cfg.TargetSuffix)
	vars := pipeline.NewVars(target)
	vars.Set("vars", t.P.Config.Variables)
	if err := pl.Index(ctx, data, vars); err != nil {
		return fmt.Errorf("index %s: %w", t.Page.RelContent, err)
	}

	log.DDebug(log.Build).
		Str("rel", t.Page.RelContent).
		Str("target", target).
		Int("bytes", len(data)).
		Msg("indexed")

	return nil
}

type BuildTask struct {
	P       *Project
	Page    PagePlan
	Tpl     *fasttemplate.Template
	Idx     func(string) pipeline.Vars
	Counter *int64
}

type ContentResult struct {
	Filename string
	Result   string // TODO: make it []byte
}

// @ts type global.tCtx
// @doc Context passed into the `transform` function.
// @pred iCtx &
// @field index: Record<string, unknown>
func (t BuildTask) Key() string { return "build:" + t.Page.RelContent }
func (t BuildTask) Run(ctx context.Context) error {
	log.TTrace(log.Build).
		Str("rel", t.Page.RelContent).
		Str("pipe", t.Page.Pipe).
		Str("layout", t.Page.LayoutKey.RelLayoutPath).
		Msg("build start")

	pl, err := t.P.pipelines.Get(t.Page.Pipe)
	if err != nil {
		return fmt.Errorf("build pipeline %s: %w", t.Page.RelContent, err)
	}

	data, err := t.P.readSource(t.Page.RelContent)
	if err != nil {
		return fmt.Errorf("build read %s: %w", t.Page.RelContent, err)
	}
	cfg := pl.Config()
	target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(t.Page.RelContent), "."+cfg.TargetSuffix)
	vars := pipeline.NewVars(target)
	vars.Set("vars", t.P.Config.Variables)

	transformed, err := pl.Transform(ctx, data, vars)
	if err != nil {
		return fmt.Errorf("transform %s: %w", t.Page.RelContent, err)
	}
	outs := make([]ContentResult, 0, len(transformed))
	for _, r := range transformed {
		if fn := r["filename"]; fn != "" {
			vars.Set(t.Page.Name, r["result"])
			out := t.Tpl.ExecuteString(vars)
			outs = append(outs, ContentResult{Filename: fn, Result: out})
		}
	}
	dir := filepath.ToSlash(t.Page.BaseDir)

	for _, r := range outs {
		dst := filepath.ToSlash(filepath.Join(dir, r.Filename))
		if err := t.P.writeFile(t.P.outRoot, dst, []byte(r.Result), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}

		log.TTrace(log.IO).
			Str("dst", dst).
			Int("bytes", len(r.Result)).
			Msg("wrote page")

	}
	if t.Counter != nil {
		atomic.AddInt64(t.Counter, int64(len(outs)))
	}

	log.DDebug(log.Build).
		Str("rel", t.Page.RelContent).
		Str("target", target).
		Int("outputs", len(outs)).
		Msg("build done")

	return nil
}

type AssetTask struct {
	P       *Project
	Asset   AssetPlan
	Counter *int64
}

func (t AssetTask) Key() string { return "asset:" + t.Asset.From + ":" + t.Asset.SourceRel }
func (t AssetTask) Run(ctx context.Context) error {
	log.TTrace(log.Build).
		Str("rel", t.Asset.SourceRel).
		Str("from", t.Asset.From).
		Str("pipe", t.Asset.PipelineID).
		Msg("asset start")

	var data []byte
	var err error
	switch t.Asset.From {
	case "source":
		data, err = t.P.readSource(t.Asset.SourceRel)
	case "scheme":
		data, err = t.P.readScheme(t.Asset.SourceRel)
	default:
		err = fmt.Errorf("unknown asset root: %s", t.Asset.From)
	}
	if err != nil {
		return err
	}
	pl, err := t.P.pipelines.Get(t.Asset.PipelineID)
	if err != nil {
		return fmt.Errorf("asset pipeline %s: %w", t.Asset.SourceRel, err)
	}

	cfg := pl.Config()
	target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(t.Asset.SourceRel), "."+cfg.TargetSuffix)

	vars := pipeline.NewVars(target)
	trs, err := pl.Transform(ctx, data, vars)
	if err != nil {
		return fmt.Errorf("asset transform %s: %w", t.Asset.SourceRel, err)
	}
	for _, r := range trs {
		dst := filepath.ToSlash(filepath.Join(t.Asset.BaseDir, r["filename"]))
		if err := t.P.writeFile(t.P.outRoot, dst, []byte(r["result"]), 0o644); err != nil {
			return fmt.Errorf("asset write %s: %w", dst, err)
		}

		log.TTrace(log.IO).
			Str("dst", dst).
			Int("bytes", len(r["result"])).
			Msg("wrote asset")

	}
	if t.Counter != nil {
		atomic.AddInt64(t.Counter, int64(len(trs)))
	}

	log.DDebug(log.Build).
		Str("rel", t.Asset.SourceRel).
		Str("from", t.Asset.From).
		Str("target", target).
		Int("outputs", len(trs)).
		Int("bytes_in", len(data)).
		Msg("asset done")

	return nil
}

type rebuildTask struct {
	s   *Server
	rel string
}

func (t rebuildTask) Key() string { return "rebuild:" + t.rel }
func (t rebuildTask) Run(ctx context.Context) error {
	log.TTrace(log.Build).Str("rel", t.rel).Msg("rebuild start")

	out, err := t.s.project.BuildPath(ctx, t.rel)
	if err != nil {
		return err
	}
	if len(out) == 0 {
		log.TTrace(log.Build).Str("rel", t.rel).Msg("rebuild noop (no outputs)")
		return nil
	}

	t.s.project.updatePlanSnapshot(t.rel)
	if t.s.config.FsWatcher.HotReload {
		newURL, _ := filepath.Rel(t.s.config.OutputPath, out[0])
		t.s.sendToClient(HotReloadMsg{Reload: false, URL: "/" + filepath.ToSlash(newURL)})
	}

	log.DDebug(log.Build).Strs("outputs", out).Msg("rebuild done")
	return nil
}
