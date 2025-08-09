package project

import (
	"context"
	"craeft/internal/log"
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
	log.Debug(fmt.Sprintf("Indexing %q", t.Page.RelContent))

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
	log.Debug(fmt.Sprintf("Building %q", t.Page.RelContent))

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
	vars.Set("index", t.Idx(t.Page.RelContent))

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
	dir := filepath.ToSlash(filepath.Join(t.Page.BaseDir))

	for _, r := range outs {
		dst := filepath.ToSlash(filepath.Join(dir, r.Filename))
		if err := t.P.writeFile(t.P.outRoot, dst, []byte(r.Result), 0o644); err != nil {
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
	log.Debug(fmt.Sprintf("Asset %q", t.Asset.SourceRel))

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
	}
	if t.Counter != nil {
		atomic.AddInt64(t.Counter, int64(len(trs)))
	}
	return nil
}

// func (p *Project) OldBuild(useCache bool) error {
// 	var assetslist = make(map[string]assetData)
//
// 	err := p.walk(func(path string, d fs.DirEntry) error {
// 		if d.IsDir() {
// 			log.Trace(fmt.Sprintf("Building folder %q", path), log.Tag{Key: log.SrcTag, Value: "layout"})
// 			currentDir := path
//
// 			layouts, err := p.getLayouts(currentDir, useCache)
// 			if err != nil {
// 				log.Error(err)
// 				return SkipFile
// 			}
//
// 			for _, lyt := range layouts {
// 				layoutTpl, err := lyt.Render(currentDir)
// 				if err != nil {
// 					log.Error(err)
// 					return SkipFile
// 				}
//
// 				for _, contentConf := range lyt.Contents() {
//
// 					log.Info(fmt.Sprintf("processing layout '%s/%s'", lyt.Path(), lyt.Name()), log.Tag{Key: log.SrcTag, Value: "layout"})
//
// 					// TODO(content): move this to content struct? see asset. content.Build(), content.Index()
// 					// TODO(important): path should not point out of the project
// 					cpath := filepath.Join(p.Config.SourcePath, currentDir, contentConf.Path)
// 					log.Debug(fmt.Sprintf("- Looking for layout content path %q", filepath.Join(currentDir, contentConf.Path)))
// 					contents, err := zglob.Glob(cpath)
// 					if err != nil {
// 						if errors.Is(err, os.ErrNotExist) {
// 							log.Warn(fmt.Sprintf("No content %s found for layout %s", filepath.Join(currentDir, contentConf.Path), lyt.Name()))
// 							return SkipFile
// 						}
// 						log.Error(err)
// 						return SkipFile
// 					}
//
// 					var contentBuffer = make(map[string]*content.Content)
//
// 					if len(contents) == 0 {
// 						log.Warn(fmt.Sprintf("layout %s/%s content path %q has no matches", lyt.Path(), lyt.Name(), contentConf.Path))
// 					}
// 					for _, contentPath := range contents {
// 						path, err := filepath.Rel(p.Config.SourcePath, contentPath)
// 						if err != nil {
// 							log.Error(err)
// 							return SkipFile
// 						}
// 						page, err := p.getContent(contentConf.Name, path, contentConf.Pipe)
// 						if err != nil {
// 							log.Error(err)
// 							return SkipFile
// 						}
// 						log.Debug(fmt.Sprintf("Indexing %q", path))
//
// 						page.LoadIndex()
// 						contentBuffer[path] = page
// 					}
// 					// }
//
// 					for _, contentPath := range contents {
// 						path, err := filepath.Rel(p.Config.SourcePath, contentPath)
// 						if err != nil {
// 							log.Error(err)
// 							return SkipFile
// 						}
// 						// page, err := p.getContent(contentPath, contentConf.Pipeline)
// 						// if err != nil {
// 						// 	return err
// 						// }
// 						page := contentBuffer[path]
//
// 						log.Debug(fmt.Sprintf("Building %q", path))
// 						results, err := page.Build(layoutTpl)
// 						if err != nil {
// 							log.Error(err)
// 							return SkipFile
// 						}
//
// 						_, err = p.writeResults(currentDir, results)
// 						if err != nil {
// 							log.Error(err)
// 							return SkipFile
// 						}
// 					}
//
// 					for _, asset := range contentConf.Assets {
// 						npath := filepath.Join(p.Config.SourcePath, asset.Source)
//
// 						log.Debug(fmt.Sprintf("found content asset %q, total path: %q", asset, npath))
// 						assetslist[npath] = assetData{pipeline: asset.AssetType, from: "content", path: npath}
// 					}
// 				}
//
// 				// assets path should be absolute!
// 				for _, asset := range lyt.Assets() {
// 					npath := filepath.Join(p.Config.SchemePath, asset.Source)
//
// 					log.Debug(fmt.Sprintf("found theme asset %q, total path: %q", asset, npath))
// 					assetslist[npath] = assetData{pipeline: asset.AssetType, from: "theme", path: npath}
// 				}
//
// 			}
// 		}
//
// 		return nil
// 	})
//
// 	log.Info(fmt.Sprintf("Building assets -> %+v", assetslist))
//
// 	for path, atype := range assetslist {
// 		var base string
// 		// atype.from says where the asset was found. inside a content or the layout
// 		if atype.from == "theme" {
// 			base, err = filepath.Rel(p.Config.SchemePath, filepath.Dir(path))
// 		} else {
// 			base, err = filepath.Rel(p.Config.SourcePath, filepath.Dir(path))
// 		}
// 		if err != nil {
// 			log.Error(err)
// 			continue
// 		}
//
// 		matches, err := zglob.Glob(path)
// 		if err != nil {
// 			log.Error(fmt.Errorf("failed building asset with path %q: %v", path, err))
// 			continue
// 		}
//
// 		for _, match := range matches {
// 			asset, err := p.getAsset(match, atype.pipeline)
// 			if err != nil {
// 				log.Error(err)
// 				continue
// 			}
//
// 			log.Debug(fmt.Sprintf("Building asset %q", match))
// 			results, err := asset.Build()
// 			if err != nil {
// 				log.Error(err)
// 				continue
// 			}
//
// 			_, err = p.writeResults(base, results)
// 			if err != nil {
// 				log.Error(err)
// 				continue
// 			}
// 		}
//
// 	}
//
// 	return err
// }
