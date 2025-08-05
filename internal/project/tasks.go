package project

import (
	"context"
	"craeft/internal/pipeline"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/valyala/fasttemplate"
)

type indexSink struct {
	mu sync.Mutex
	m  map[string]pipeline.Vars
}

func newIndexSink(n int) *indexSink                { return &indexSink{m: make(map[string]pipeline.Vars, n)} }
func (s *indexSink) set(k string, v pipeline.Vars) { s.mu.Lock(); s.m[k] = v; s.mu.Unlock() }
func (s *indexSink) get(k string) pipeline.Vars    { s.mu.Lock(); v := s.m[k]; s.mu.Unlock(); return v }

type IndexTask struct {
	P    *Project
	Page PagePlan
	Sink *indexSink
}

func (t IndexTask) Key() string { return "index:" + t.Page.RelContent }
func (t IndexTask) Run(ctx context.Context) error {
	pl, _ := t.P.pipelines.Get(t.Page.Pipe)
	data, err := t.P.readSource(t.Page.RelContent)
	if err != nil {
		return fmt.Errorf("index read %s: %w", t.Page.RelContent, err)
	}
	cfg := pl.Config()
	target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(t.Page.RelContent), "."+cfg.TargetSuffix)
	vars := pipeline.NewVars(target)
	vars.Set("vars", t.P.Config.Variables)
	iv, err := pl.Index(ctx, data, vars)
	if err != nil {
		return fmt.Errorf("index %s: %w", t.Page.RelContent, err)
	}
	t.Sink.set(t.Page.RelContent, iv)
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

func (t BuildTask) Key() string { return "build:" + t.Page.RelContent }
func (t BuildTask) Run(ctx context.Context) error {
	pl, _ := t.P.pipelines.Get(t.Page.Pipe)
	data, err := t.P.readSource(t.Page.RelContent)
	if err != nil {
		return fmt.Errorf("build read %s: %w", t.Page.RelContent, err)
	}
	cfg := pl.Config()
	target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(t.Page.RelContent), "."+cfg.TargetSuffix)
	vars := pipeline.NewVars(target)
	vars.Set("vars", t.P.Config.Variables)
	vars.Set("index", t.Idx(t.Page.RelContent)) // per-page only

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
	// write into output root, preserving relative base dir
	dir := filepath.ToSlash(filepath.Join(t.Page.BaseDir))
	for _, r := range outs {
		dst := filepath.ToSlash(filepath.Join(dir, r.Filename))
		if err := t.P.writeFileAtomic(t.P.outRoot, dst, []byte(r.Result), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}
	if t.Counter != nil {
		atomic.AddInt64(t.Counter, int64(len(outs)))
	}
	return nil
}

type AssetTask struct {
	P       *Project
	Asset   AssetPlan
	Counter *int64
}

func (t AssetTask) Key() string { return "asset:" + t.Asset.From + ":" + t.Asset.SourceRel }
func (t AssetTask) Run(ctx context.Context) error {
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
	pl, _ := t.P.pipelines.Get(t.Asset.PipelineID)
	cfg := pl.Config()
	target := cfg.SourceSuffixRe.ReplaceAllString(filepath.Base(t.Asset.SourceRel), "."+cfg.TargetSuffix)
	vars := pipeline.NewVars(target)
	trs, err := pl.Transform(ctx, data, vars)
	if err != nil {
		return fmt.Errorf("asset transform %s: %w", t.Asset.SourceRel, err)
	}
	for _, r := range trs {
		dst := filepath.ToSlash(filepath.Join(t.Asset.BaseDir, r["filename"]))
		if err := t.P.writeFileAtomic(t.P.outRoot, dst, []byte(r["result"]), 0o644); err != nil {
			return fmt.Errorf("asset write %s: %w", dst, err)
		}
	}
	if t.Counter != nil {
		atomic.AddInt64(t.Counter, int64(len(trs)))
	}
	return nil
}
