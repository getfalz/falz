package project

import (
	"io/fs"
	"path/filepath"
	"time"

	"craeft/internal/log"
	"craeft/internal/utils"
)

type PagePlan struct {
	BaseDir    string
	RelContent string
	Name       string
	Pipe       string
	LayoutKey  LayoutKey
}

type AssetPlan struct {
	BaseDir    string
	SourceRel  string // relative to SourcePath or SchemePath (see SourceFrom)
	From       string // "source" | "scheme"
	PipelineID string
}

type Plan struct {
	PagesByLayout map[LayoutKey][]PagePlan
	Assets        map[assetKey]AssetPlan
}

func (p *Project) plan() (Plan, error) {
	start := time.Now()
	log.IInfo(log.Plan).Msg("planning started")
	srcFS := p.srcRoot.FS()
	res := Plan{
		PagesByLayout: make(map[LayoutKey][]PagePlan),
		Assets:        make(map[assetKey]AssetPlan),
	}

	var pagesAdded int
	err := fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if p.ignoreFile(path) {
				return fs.SkipDir
			}
			base := filepath.ToSlash(path)

			lmap, err := p.getLayouts(base)
			if err != nil {
				log.TTrace(log.Layout).Str("base", base).Err(err).Msg("no layouts for base")
				return nil
			}
			for _, lyt := range lmap {
				key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}

				for _, cc := range lyt.Contents() {
					glob := filepath.ToSlash(filepath.Join(base, cc.Path))
					matches, err := fs.Glob(p.srcRoot.FS(), glob)
					if err != nil {
						continue
					}
					for _, rel := range matches {
						res.PagesByLayout[key] = append(res.PagesByLayout[key], PagePlan{
							BaseDir: base, RelContent: rel, Name: cc.Name, Pipe: cc.Pipe, LayoutKey: key,
						})
						pagesAdded++
						log.TTrace(log.Plan).
							Str("layout", key.RelLayoutPath).
							Str("content", filepath.ToSlash(rel)).
							Str("pipe", cc.Pipe).
							Msg("page planned")
					}
					for _, a := range cc.Assets() {
						glob := filepath.ToSlash(filepath.Join(base, a.Source))
						matches, err := fs.Glob(p.srcRoot.FS(), glob)
						if err != nil {
							log.TTrace(log.Plan).Str("glob", glob).Err(err).Msg("content asset glob failed")
							continue
						}

						for _, rel := range matches {
							k := assetKey{From: "source", Pipe: a.AssetType, Rel: utils.NormRel(rel)}
							res.Assets[k] = AssetPlan{
								BaseDir:    filepath.Dir(rel),
								SourceRel:  rel,
								From:       "source",
								PipelineID: a.AssetType,
							}
							log.TTrace(log.Plan).
								Str("from", "source").
								Str("pipe", a.AssetType).
								Str("rel", filepath.ToSlash(rel)).
								Msg("asset planned")
						}
					}
				}

				for _, a := range lyt.Assets() {
					glob := filepath.ToSlash(filepath.Join(lyt.Path(), a.Source))
					matches, err := fs.Glob(p.schemeRoot.FS(), glob)
					if err != nil {
						continue
					}

					for _, rel := range matches {
						k := assetKey{From: "scheme", Pipe: a.AssetType, Rel: utils.NormRel(rel)}
						res.Assets[k] = AssetPlan{
							BaseDir:    filepath.Dir(rel),
							SourceRel:  rel,
							From:       "scheme",
							PipelineID: a.AssetType,
						}
						log.TTrace(log.Plan).
							Str("from", "scheme").
							Str("pipe", a.AssetType).
							Str("rel", filepath.ToSlash(rel)).
							Msg("asset planned")
					}
				}
			}
		}
		return nil
	})
	p.setLastPlan(res)
	log.DDebug(log.Plan).
		Int("pages_planned", pagesAdded).
		Int("assets_planned", len(res.Assets)).
		Int("layouts", len(res.PagesByLayout)).
		Dur("duration", time.Since(start)).
		Msg("planning finished")
	return res, err
}

type planSnapshot struct {
	pages  map[LayoutKey][]PagePlan
	assets map[assetKey]AssetPlan
}

func (p *Project) setLastPlan(pl Plan) {
	p.lastPlanMu.Lock()
	p.lastPlan = planSnapshot{
		pages:  pl.PagesByLayout,
		assets: pl.Assets,
	}
	p.lastPlanMu.Unlock()

	log.DDebug(log.Plan).
		Int("layouts", len(pl.PagesByLayout)).
		Int("assets", len(pl.Assets)).
		Msg("last plan snapshot updated")
}

func (p *Project) snapshotPlan() planSnapshot {
	p.lastPlanMu.RLock()
	defer p.lastPlanMu.RUnlock()
	// shallow copy maps to avoid external mutation
	pages := make(map[LayoutKey][]PagePlan, len(p.lastPlan.pages))
	for k, v := range p.lastPlan.pages {
		vc := make([]PagePlan, len(v))
		copy(vc, v)
		pages[k] = vc
	}
	assets := make(map[assetKey]AssetPlan, len(p.lastPlan.assets))
	for k, v := range p.lastPlan.assets {
		assets[k] = v
	}

	snap := planSnapshot{pages: pages, assets: assets}

	log.TTrace(log.Plan).
		Int("pages_groups", len(snap.pages)).
		Int("assets", len(snap.assets)).
		Msg("got snapshot")

	return snap
}

func (p *Project) updatePlanSnapshot(rel string) {
	rel = filepath.ToSlash(rel)
	base := filepath.ToSlash(filepath.Dir(rel))
	name := filepath.Base(rel)

	lmap, _ := p.getLayouts(base)

	log.DDebug(log.Plan).Str("rel", rel).Str("base", base).Msg("updating snapshot for path")

	type m struct{ ldir, nm, pipe string }
	var matches []m
	for ldir, lyt := range lmap {
		if !lyt.MatchContent(name) {
			continue
		}
		for _, c := range lyt.Contents() {
			if c.MatchContent(name) {
				matches = append(matches, m{ldir, c.Name, c.Pipe})
			}
		}
	}

	var newPlan []PagePlan
	for _, mm := range matches {
		key := LayoutKey{RelLayoutPath: mm.ldir, BaseDir: base}
		newPlan = append(newPlan, PagePlan{
			BaseDir:    base,
			RelContent: rel,
			Name:       mm.nm,
			Pipe:       mm.pipe,
			LayoutKey:  key,
		})
	}

	p.lastPlanMu.Lock()
	defer p.lastPlanMu.Unlock()

	var removed int
	for k, arr := range p.lastPlan.pages {
		// reuse the slice to avoid allocations.
		// reset the slice to zero length
		w := arr[:0]
		for _, pp := range arr {
			if filepath.ToSlash(pp.RelContent) != rel {
				w = append(w, pp)
			}
		}
		if len(w) == 0 {
			delete(p.lastPlan.pages, k)
			removed++
		} else {
			p.lastPlan.pages[k] = w
		}
	}
	for _, pp := range newPlan {
		p.lastPlan.pages[pp.LayoutKey] = append(p.lastPlan.pages[pp.LayoutKey], pp)
	}

	log.DDebug(log.Plan).
		Str("rel", rel).
		Int("removed_groups", removed).
		Int("added", len(newPlan)).
		Msg("snapshot updated")
}

func (p *Project) AffectedByPipeline(name string) []string {
	snap := p.snapshotPlan()
	seen := make(map[string]struct{})
	var out []string
	for _, pages := range snap.pages {
		for _, pg := range pages {
			if pg.Pipe != name {
				continue
			}
			r := filepath.ToSlash(pg.RelContent)
			if _, ok := seen[r]; ok {
				continue
			}
			seen[r] = struct{}{}
			out = append(out, r)
		}
	}
	log.DDebug(log.Plan).Str("pipeline", name).Int("affected", len(out)).Msg("affected by pipeline")
	return out
}

func (p *Project) AffectedByLayoutKeys(keys []LayoutKey) []string {
	if len(keys) == 0 {
		log.TTrace(log.Plan).Int("keys", 0).Msg("affected by layouts (none)")
		return nil
	}

	snap := p.snapshotPlan()
	seen := make(map[string]struct{})
	out := make([]string, 0, 32)

	for _, k := range keys {
		pages := snap.pages[k]
		for _, pg := range pages {
			r := filepath.ToSlash(pg.RelContent)
			if _, dup := seen[r]; dup {
				continue
			}
			seen[r] = struct{}{}
			out = append(out, r)
		}
	}
	log.DDebug(log.Plan).Int("keys", len(keys)).Int("affected", len(out)).Msg("affected by layout keys")
	return out
}
