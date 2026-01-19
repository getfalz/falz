package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"craeft/internal/log"
	"craeft/internal/project"
	"craeft/internal/utils"
)

func (s *Server) handleContentModified(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {

		if !plan.HasEntry(path) {
			log.DDebug(log.Handler).
				Str("path", path).
				Msg("content modified but not in plan")
			continue
		}

		newHash, err := s.project.SourceFS.HashFile(path)
		if err != nil {
			log.Warn(fmt.Sprintf("hash content %s: %v", path, err))
			return nil
		}

		_ = plan.UpdateEntry(path, func(entry *project.BuildEntry) error {
			if entry.ContentHash != newHash {
				oldHash := entry.ContentHash
				entry.ContentHash = newHash

				entry.Hash = utils.CombineHashes(
					entry.ContentHash,
					entry.LayoutHash,
					entry.PipelineHash,
					utils.CombineHashes(entry.FragmentHashes...),
				)
				entry.Dirty = true

				log.DDebug(log.Handler).
					Str("id", entry.ID).
					Str("old_hash", oldHash[:8]).
					Str("new_hash", newHash[:8]).
					Msg("content changed, marked for rebuild")

			}
			return nil
		})
	}
	return nil
}

func (s *Server) handleContentCreated(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		if plan.HasEntry(path) {
			continue
		}

		base := filepath.Dir(path)

		lmap, err := s.project.GetLayouts(base)
		if err != nil {
			log.TTrace(log.Handler).
				Str("path", path).
				Err(err).
				Msg("no layouts for new content")
			continue
		}

		for _, lyt := range lmap {
			key := project.LayoutKey{
				RelLayoutPath: lyt.Path(),
				BaseDir:       base,
			}

			for _, cc := range lyt.Contents() {
				glob := filepath.ToSlash(filepath.Join(base, cc.Source))
				matches, err := s.project.SourceFS.Glob(glob)
				if err != nil || len(matches) == 0 {
					continue
				}

				_, err = s.project.Layouts.GetOrBuild(ctx, s.project.SchemeFS, key, lyt)
				if err != nil {
					log.Warn(fmt.Sprintf("warm up layout for new content %s: %v", path, err))
					continue
				}

				// entry, err := s.project.CreateEntryWithHash(ctx, plan, "content", "source", path, cc.To, base, cc.Pipe, cc.Name, lyt)
				// if err != nil {
				// 	log.Warn(fmt.Sprintf("create content asset %s: %v", path, err))
				// 	continue
				// }
				//
				// entry.Dirty = true
				// plan.AddEntry(entry.ID, entry)
				spec := project.EntrySpec{
					Type:       project.EntryTypeContent,
					SourceRoot: project.SourceTypeSource,
					Rel:        path,
					BaseDir:    base,
					Pipeline:   cc.Pipe,
					TagName:    cc.Name,
					ToTpl:      cc.To,
					Layout:     lyt,
					LayoutKey:  project.LayoutKey{RelLayoutPath: lyt.Path(), BaseDir: base},
				}
				// TODO handle ok
				entry, _ := s.project.UpsertEntry(plan, spec)

				log.IInfo(log.Handler).
					Str("id", entry.ID).
					Str("layout", key.RelLayoutPath).
					Msg("new content added to plan")
			}
		}
	}
	return nil
}

func (s *Server) handleContentDeleted(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		if plan.HasEntry(path) {
			plan.DeleteEntry(path)
			plan.Manifest.Delete(path)

			log.IInfo(log.Handler).
				Str("id", path).
				Msg("content deleted from plan")
		}
	}
	return nil
}

// Content Asset handlers
func (s *Server) handleContentAssetModified(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		id := fmt.Sprintf("asset:source:%s", path)
		if !plan.HasEntry(id) {
			log.DDebug(log.Handler).
				Str("path", path).
				Msg("asset modified but not in plan")
			continue
		}

		newHash, err := s.project.SourceFS.HashFile(path)
		if err != nil {
			log.Warn(fmt.Sprintf("hash asset %s: %v", path, err))
			continue
		}

		_ = plan.UpdateEntry(id, func(entry *project.BuildEntry) error {
			if entry.ContentHash != newHash {
				entry.ContentHash = newHash

				entry.Hash = utils.CombineHashes(
					entry.ContentHash,
					entry.LayoutHash,
					entry.PipelineHash,
					utils.CombineHashes(entry.FragmentHashes...),
				)

				entry.Dirty = true

				log.DDebug(log.Handler).
					Str("id", entry.ID).
					Msg("asset changed, marked for rebuild")
			}
			return nil
		})
	}
	return nil
}

func (s *Server) handleContentAssetCreated(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		id := fmt.Sprintf("asset:source:%s", path)

		if plan.HasEntry(id) {
			continue
		}

		dir := filepath.Dir(path)

		lmap, err := s.project.GetLayouts(dir)
		if err != nil {
			continue
		}

		for _, lyt := range lmap {
			for _, cc := range lyt.Contents() {
				for _, a := range cc.Assets() {
					glob := filepath.ToSlash(filepath.Join(dir, a.Source))
					matches, err := s.project.SourceFS.Glob(glob)
					if err != nil || len(matches) == 0 {
						continue
					}

					// entry, err := s.project.CreateEntryWithHash(ctx, plan, "asset", "source", path, a.To, filepath.Dir(path), a.Pipe, a.Name, nil)
					// if err != nil {
					// 	log.Warn(fmt.Sprintf("create content asset %s: %v", path, err))
					// 	continue
					// }
					//
					// entry.Dirty = true
					// plan.AddEntry(entry.ID, entry)
					spec := project.EntrySpec{
						Type:       project.EntryTypeAsset,
						SourceRoot: project.SourceTypeSource,
						Rel:        path,
						BaseDir:    filepath.Base(path),
						ToTpl:      a.To,
						Pipeline:   a.Pipe,
					}
					// TODO handle ok
					entry, _ := s.project.UpsertEntry(plan, spec)

					log.IInfo(log.Handler).
						Str("id", entry.ID).
						Str("pipeline", a.Pipe).
						Msg("new asset added to plan")
				}
			}
		}
	}
	return nil
}

func (s *Server) handleContentAssetDeleted(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		id := fmt.Sprintf("asset:source:%s", path)
		if plan.HasEntry(id) {
			plan.DeleteEntry(id)
			plan.Manifest.Delete(id)

			log.IInfo(log.Handler).
				Str("id", id).
				Msg("asset deleted from plan")
		}
	}
	return nil
}

// Layout handlers

func (s *Server) handleLayoutModified(ctx context.Context, plan *project.Plan, paths []string) error {
	var keys []project.LayoutKey

	for _, path := range paths {
		dir := filepath.Dir(path)

		log.IInfo(log.Handler).
			Str("layout", path).
			Str("dir", dir).
			Msg("layout modified, rescanning directory")

		affectedKeys := plan.EntryKeys(func(e *project.BuildEntry) bool {
			return e.Type == "content" &&
				filepath.ToSlash(e.LayoutKey.RelLayoutPath) == filepath.ToSlash(path)
		})

		for _, entryID := range affectedKeys {
			if entry, ok := plan.Entry(entryID); ok {
				// s.project.Layouts.InvalidateByKey(entry.LayoutKey)
				keys = append(keys, entry.LayoutKey)
			}
		}

		log.DDebug(log.Handler).
			Str("layout", path).
			Int("affected_keys", len(affectedKeys)).
			Msg("layout cache invalidated")

	}
	return s.project.InvalidateLayouts(ctx, plan, keys, true)
}

func (s *Server) handleLayoutCreated(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		dir := filepath.Dir(path)

		log.IInfo(log.Handler).
			Str("layout", path).
			Str("dir", dir).
			Msg("new layout created, rescanning directory")

		// Remove entries that were using parent layout
		plan.RemoveIf(func(entry *project.BuildEntry) bool {
			return entry.Type == "content" && strings.HasPrefix(entry.BaseDir, dir)
		}, false)

		// if err := s.project.PartialScan(ctx, plan, dir); err != nil {
		// 	log.EError(log.Handler).
		// 		Err(err).
		// 		Str("dir", dir).
		// 		Msg("partial scan failed")
		// }

		if _, err := s.project.Scan(ctx, plan, dir, false); err != nil {
			log.EError(log.Handler).
				Err(err).
				Str("dir", dir).
				Msg("scan failed")
		} else {
			if _, err := s.project.HashDirty(ctx, plan); err != nil {
				log.EError(log.Handler).Err(err).Str("dir", dir).Msg("hash dirty failed")
			}
		}
	}
	return nil
}

func (s *Server) handleLayoutDeleted(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		dir := filepath.Dir(path)

		log.IInfo(log.Handler).
			Str("layout", path).
			Str("dir", dir).
			Msg("layout deleted, rescanning with parent layout")

		affectedKeys := s.project.Layouts.InvalidateAndGetAffected(path)

		affectedKeysMap := make(map[project.LayoutKey]struct{})
		for _, key := range affectedKeys {
			affectedKeysMap[key] = struct{}{}
		}

		plan.RemoveIf(func(entry *project.BuildEntry) bool {
			if entry.Type != "content" {
				return false
			}
			_, found := affectedKeysMap[entry.LayoutKey]
			return found
		}, true)

		// if err := s.project.PartialScan(ctx, plan, dir); err != nil {
		// 	log.EError(log.Handler).
		// 		Err(err).
		// 		Str("dir", dir).
		// 		Msg("partial scan failed")
		// }

		if _, err := s.project.Scan(ctx, plan, dir, false); err != nil {
			log.EError(log.Handler).
				Err(err).
				Str("dir", dir).
				Msg("scan failed")
		} else {
			if _, err := s.project.HashDirty(ctx, plan); err != nil {
				log.EError(log.Handler).Err(err).Str("dir", dir).Msg("hash dirty failed")
			}
		}
	}
	return nil
}

// Fragment handler

func (s *Server) handleFragmentModified(ctx context.Context, plan *project.Plan, paths []string) error {
	var keys []project.LayoutKey
	for _, path := range paths {
		affectedKeys := s.project.Layouts.InvalidateAndGetAffected(path)
		keys = append(keys, affectedKeys...)

		log.DDebug(log.Handler).
			Str("fragment", path).
			Int("affected_keys", len(affectedKeys)).
			Msg("fragment modified")
	}

	return s.project.InvalidateLayouts(ctx, plan, keys, false)
}

// Scheme Asset handlers

func (s *Server) handleSchemeAssetModified(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		id := fmt.Sprintf("asset:scheme:%s", path)

		if !plan.HasEntry(id) {
			log.DDebug(log.Handler).
				Str("path", path).
				Msg("scheme asset modified but not in plan")
			continue
		}

		newHash, err := s.project.SchemeFS.HashFile(path)
		if err != nil {
			log.Warn(fmt.Sprintf("hash scheme asset %s: %v", path, err))
			continue
		}

		plan.UpdateEntry(id, func(entry *project.BuildEntry) error {
			if entry.ContentHash != newHash {
				entry.ContentHash = newHash

				entry.Hash = utils.CombineHashes(
					entry.ContentHash,
					entry.LayoutHash,
					entry.PipelineHash,
					utils.CombineHashes(entry.FragmentHashes...),
				)

				entry.Dirty = true

				log.DDebug(log.Handler).
					Str("id", entry.ID).
					Msg("scheme asset changed, marked for rebuild")
			}

			return nil
		})
	}
	return nil
}

func (s *Server) handleSchemeAssetCreated(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		dir := filepath.Dir(path)

		lmap, err := s.project.GetLayouts(dir)
		if err != nil {
			continue
		}

		for _, lyt := range lmap {
			for _, a := range lyt.Assets() {
				glob := filepath.ToSlash(filepath.Join(dir, a.Source))

				matches, err := s.project.SchemeFS.Glob(glob)
				if err != nil || len(matches) == 0 {
					continue
				}

				// entry, err := s.project.CreateEntryWithHash(ctx, plan, "asset", "scheme", path, a.To, filepath.Dir(path), a.Pipe, a.Name, nil)
				// if err != nil {
				// 	log.Warn(fmt.Sprintf("create scheme asset %s: %v", path, err))
				// 	continue
				// }
				//
				// entry.Dirty = true
				// plan.AddEntry(entry.ID, entry)
				spec := project.EntrySpec{
					Type:       project.EntryTypeAsset,
					SourceRoot: project.SourceTypeScheme,
					Rel:        path,
					BaseDir:    filepath.Dir(path),
					ToTpl:      a.To,
					Pipeline:   a.Pipe,
				}
				// TODO handle ok
				entry, _ := s.project.UpsertEntry(plan, spec)

				log.IInfo(log.Handler).
					Str("id", entry.ID).
					Str("pipeline", a.Pipe).
					Msg("new scheme asset added to plan")
			}
		}
	}
	return nil
}

func (s *Server) handleSchemeAssetDeleted(ctx context.Context, plan *project.Plan, paths []string) error {
	for _, path := range paths {
		id := fmt.Sprintf("asset:scheme:%s", path)

		if plan.HasEntry(id) {
			plan.DeleteEntry(id)
			plan.Manifest.Delete(id)

			log.DDebug(log.Handler).
				Str("id", id).
				Msg("scheme asset deleted from plan")
		}
	}
	return nil
}

// Pipeline handler

// TODO this method need rework
func (s *Server) handlePipelineModified(ctx context.Context, plan *project.Plan, paths []string) error {
	pipelineNames := make(map[string]struct{})
	for _, path := range paths {
		parts := strings.Split(path, "/")
		if len(parts) > 0 {
			pipelineNames[parts[0]] = struct{}{}
		}
	}
	fmt.Printf("TODO: pipeline modified: %v\n", pipelineNames)

	for pipeName := range pipelineNames {
		if err := s.project.Pipes.Reload(pipeName); err != nil {
			log.Warn(fmt.Sprintf("pipeline reload failed: %s: %v", pipeName, err))
			continue
		}

		log.IInfo(log.Handler).Str("pipeline", pipeName).Msg("reloaded")

		pl, err := s.project.Pipes.Get(pipeName)
		if err != nil {
			continue
		}

		cfg := pl.Config()
		newPipelineHash := utils.HashString(fmt.Sprintf("%s-%s-%s",
			cfg.PipelineName, cfg.SourceSuffix, cfg.TargetSuffix))

		keys := plan.EntryKeys(func(e *project.BuildEntry) bool {
			// TODO rework pipe hash?
			// reload on any file change in pipe. ignore hash
			return e.Pipeline == pipeName // && e.PipelineHash != newPipelineHash
		})
		for _, k := range keys {
			plan.UpdateEntry(k, func(entry *project.BuildEntry) error {

				entry.PipelineHash = newPipelineHash

				entry.Hash = utils.CombineHashes(
					entry.ContentHash,
					entry.LayoutHash,
					entry.PipelineHash,
					utils.CombineHashes(entry.FragmentHashes...),
				)

				entry.Dirty = true

				log.DDebug(log.Handler).
					Str("id", entry.ID).
					Str("pipeline", pipeName).
					Msg("pipeline changed, entry marked for rebuild")

				return nil
			})
		}
	}
	return nil
}
