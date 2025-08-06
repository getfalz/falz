package project

import (
	"craeft/internal/utils"
	"io/fs"
	"path/filepath"
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
	srcFS := p.srcRoot.FS()
	res := Plan{
		PagesByLayout: make(map[LayoutKey][]PagePlan),
		Assets:        make(map[assetKey]AssetPlan),
	}

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
					}
					for _, a := range cc.Assets {
						glob := filepath.ToSlash(filepath.Join(base, a.Source))
						matches, err := fs.Glob(p.srcRoot.FS(), glob)
						if err != nil {
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
					}
				}
			}
		}
		return nil
	})
	return res, err
}

func baseFrom(baseDir, src string) string {
	dir := filepath.Dir(filepath.ToSlash(src))
	if dir == "." {
		return baseDir
	}
	return filepath.ToSlash(filepath.Join(baseDir, dir))
}
