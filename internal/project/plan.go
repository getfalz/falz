package project

import (
	"io/fs"
	"path/filepath"

	"github.com/mattn/go-zglob"
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
	Assets        []AssetPlan
}

//	func (p *Project) Build(useCache bool) error {
//		var assetslist = make(map[string]assetData)
//
//		err := p.walk(func(path string, d fs.DirEntry) error {
//			if d.IsDir() {
//				log.Trace(fmt.Sprintf("Building folder %q", path), log.Tag{Key: log.SrcTag, Value: "layout"})
//				currentDir := path
//
//				layouts, err := p.getLayouts(currentDir, useCache)
//				if err != nil {
//					log.Error(err)
//					return SkipFile
//				}
//
//				for _, lyt := range layouts {
//					layoutTpl, err := lyt.Render(currentDir)
//					if err != nil {
//						log.Error(err)
//						return SkipFile
//					}
//
//					for _, contentConf := range lyt.Contents() {
//
//						log.Info(fmt.Sprintf("processing layout '%s/%s'", lyt.Path(), lyt.Name()), log.Tag{Key: log.SrcTag, Value: "layout"})
//
//						// TODO(content): move this to content struct? see asset. content.Build(), content.Index()
//						// TODO(important): path should not point out of the project
//						cpath := filepath.Join(p.Config.SourcePath, currentDir, contentConf.Path)
//						log.Debug(fmt.Sprintf("- Looking for layout content path %q", filepath.Join(currentDir, contentConf.Path)))
//						contents, err := zglob.Glob(cpath)
//						if err != nil {
//							if errors.Is(err, os.ErrNotExist) {
//								log.Warn(fmt.Sprintf("No content %s found for layout %s", filepath.Join(currentDir, contentConf.Path), lyt.Name()))
//								return SkipFile
//							}
//							log.Error(err)
//							return SkipFile
//						}
//
//						var contentBuffer = make(map[string]*content.Content)
//
//						if len(contents) == 0 {
//							log.Warn(fmt.Sprintf("layout %s/%s content path %q has no matches", lyt.Path(), lyt.Name(), contentConf.Path))
//						}
//						for _, contentPath := range contents {
//							path, err := filepath.Rel(p.Config.SourcePath, contentPath)
//							if err != nil {
//								log.Error(err)
//								return SkipFile
//							}
//							page, err := p.getContent(contentConf.Name, path, contentConf.Pipe)
//							if err != nil {
//								log.Error(err)
//								return SkipFile
//							}
//							log.Debug(fmt.Sprintf("Indexing %q", path))
//
//							page.LoadIndex()
//							contentBuffer[path] = page
//						}
//						// }
//
//						for _, contentPath := range contents {
//							path, err := filepath.Rel(p.Config.SourcePath, contentPath)
//							if err != nil {
//								log.Error(err)
//								return SkipFile
//							}
//							// page, err := p.getContent(contentPath, contentConf.Pipeline)
//							// if err != nil {
//							// 	return err
//							// }
//							page := contentBuffer[path]
//
//							log.Debug(fmt.Sprintf("Building %q", path))
//							results, err := page.Build(layoutTpl)
//							if err != nil {
//								log.Error(err)
//								return SkipFile
//							}
//
//							_, err = p.writeResults(currentDir, results)
//							if err != nil {
//								log.Error(err)
//								return SkipFile
//							}
//						}
//
//						for _, asset := range contentConf.Assets {
//							npath := filepath.Join(p.Config.SourcePath, asset.Source)
//
//							log.Debug(fmt.Sprintf("found content asset %q, total path: %q", asset, npath))
//							assetslist[npath] = assetData{pipeline: asset.AssetType, from: "content", path: npath}
//						}
//					}
//
//					// assets path should be absolute!
//					for _, asset := range lyt.Assets() {
//						npath := filepath.Join(p.Config.SchemePath, asset.Source)
//
//						log.Debug(fmt.Sprintf("found theme asset %q, total path: %q", asset, npath))
//						assetslist[npath] = assetData{pipeline: asset.AssetType, from: "theme", path: npath}
//					}
//
//				}
//			}
//
//			return nil
//		})
//
//		log.Info(fmt.Sprintf("Building assets -> %+v", assetslist))
//
//		for path, atype := range assetslist {
//			var base string
//			// atype.from says where the asset was found. inside a content or the layout
//			if atype.from == "theme" {
//				base, err = filepath.Rel(p.Config.SchemePath, filepath.Dir(path))
//			} else {
//				base, err = filepath.Rel(p.Config.SourcePath, filepath.Dir(path))
//			}
//			if err != nil {
//				log.Error(err)
//				continue
//			}
//
//			matches, err := zglob.Glob(path)
//			if err != nil {
//				log.Error(fmt.Errorf("failed building asset with path %q: %v", path, err))
//				continue
//			}
//
//			for _, match := range matches {
//				asset, err := p.getAsset(match, atype.pipeline)
//				if err != nil {
//					log.Error(err)
//					continue
//				}
//
//				log.Debug(fmt.Sprintf("Building asset %q", match))
//				results, err := asset.Build()
//				if err != nil {
//					log.Error(err)
//					continue
//				}
//
//				_, err = p.writeResults(base, results)
//				if err != nil {
//					log.Error(err)
//					continue
//				}
//			}
//
//		}
//
//		return err
//	}
//

func (p *Project) plan(useCache bool) (Plan, error) {
	srcFS := p.srcRoot.FS()
	res := Plan{PagesByLayout: make(map[LayoutKey][]PagePlan)}

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
				return nil // best-effort: skip this dir
			}
			for _, lyt := range lmap {
				key := LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base}

				// page contents
				for _, cc := range lyt.Contents() {
					glob := filepath.ToSlash(filepath.Join(base, cc.Path))
					matches, err := zglob.Glob(glob) // works on slash paths
					if err != nil {
						continue
					}
					for _, rel := range matches {
						res.PagesByLayout[key] = append(res.PagesByLayout[key], PagePlan{
							BaseDir: base, RelContent: rel, Name: cc.Name, Pipe: cc.Pipe, LayoutKey: key,
						})
					}
					// content assets
					for _, a := range cc.Assets {
						res.Assets = append(res.Assets, AssetPlan{
							BaseDir:    baseFrom(base, a.Source),
							SourceRel:  filepath.ToSlash(a.Source),
							From:       "source",
							PipelineID: a.AssetType,
						})
					}
				}

				// layout/theme assets (come from scheme root)
				for _, a := range lyt.Assets() {
					res.Assets = append(res.Assets, AssetPlan{
						BaseDir:    baseFrom(".", a.Source), // relative to scheme root path segment
						SourceRel:  filepath.ToSlash(a.Source),
						From:       "scheme",
						PipelineID: a.AssetType,
					})
				}
			}
		}
		return nil
	})
	return res, err
}

func baseFrom(baseDir, src string) string {
	// outputs mirror the directory that contained the asset; keep simple here
	dir := filepath.Dir(filepath.ToSlash(src))
	if dir == "." {
		return baseDir
	}
	return filepath.ToSlash(filepath.Join(baseDir, dir))
}
