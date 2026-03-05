package project

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"craeft/internal/layout"
	"craeft/internal/log"
)

type SourceIndex struct {
	fs *FSRegistry
	lc LayoutRegistry

	ignore []string
}

type SourceIndexConfig struct {
	Ignore []string
}

func NewSourceIndex(fs *FSRegistry, lc LayoutRegistry, config SourceIndexConfig) *SourceIndex {
	return &SourceIndex{
		fs: fs,
		lc: lc,

		ignore: config.Ignore,
	}
}

// Scan performs a full scan of the entire source tree.
// Used during initial build.
func (si *SourceIndex) Scan(ctx context.Context, graph DependencyGraph) error {

	return fs.WalkDir(si.fs.Source().FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !d.IsDir() {
			return nil
		}
		if si.IgnoreFile(path) {
			return fs.SkipDir
		}

		base := filepath.ToSlash(path)
		return si.scanDirectory(ctx, graph, base)
	})
}

// ScanDirectory performs a partial scan of a specific directory.
// Stops at subdirectories that have their own layouts.
// Used in watch mode when a directory is created or layout is added.
func (si *SourceIndex) PartialScan(ctx context.Context, graph DependencyGraph, baseDir string) error {

	return fs.WalkDir(si.fs.Source().FS(), baseDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !d.IsDir() {
			return nil
		}
		if si.IgnoreFile(path) {
			return fs.SkipDir
		}

		// Stop at subdirs with layouts (partial scan only)
		if path != baseDir {
			lmap, err := si.lc.LayoutsInDir(ctx, path)
			if len(lmap) > 0 {
				return fs.SkipDir
			}

			if err != nil {
				// TODO skip dir and ignore error?
				return fs.SkipDir
			}
		}

		base := filepath.ToSlash(path)
		return si.scanDirectory(ctx, graph, base)
	})
}

func (si *SourceIndex) scanDirectory(ctx context.Context, graph DependencyGraph, path string) error {

	lkeys, err := si.lc.LayoutsForDir(ctx, path)
	if err != nil {
		log.WWarn(log.Plan).Err(err).Str("rel", path).Msg("failed to load layout")
		return nil
	}

	for lytPath, lKey := range lkeys {
		lyt, err := si.lc.Parse(lKey)
		if err != nil {
			return fmt.Errorf("failed to parse layout %q: %w", lytPath, err)
		}

		lytNode := NewLayoutNode(lKey)
		graph.AddNode(lytNode)

		for _, frag := range lyt.Fragments() {
			fragPaths, err := si.fs.Scheme().FindFile(path, frag.Source)
			if err != nil {
				return err
			}

			fragNode := NewFragmentNode(fragPaths[0])
			graph.AddNode(fragNode)
			graph.AddEdge(lytNode, fragNode)
		}

		if lyt.TplPath != "" {
			tplPaths, err := si.fs.Scheme().FindFile(path, lyt.TplPath)
			if err != nil {
				return err
			}

			tplNode := NewTemplateNode(tplPaths[0])
			graph.AddNode(tplNode)
			graph.AddEdge(lytNode, tplNode)
		}

		for _, content := range lyt.Contents() {
			err := si.scanContentPattern(ctx, graph, path, lKey, content)
			if err != nil {
				continue
			}
		}

		for _, asset := range lyt.Assets() {
			err := si.scanAssetPattern(ctx, graph, path, lKey, asset)
			if err != nil {
				continue
			}
		}

	}

	return nil
}

func (si *SourceIndex) scanContentPattern(ctx context.Context, graph DependencyGraph, base string, key LayoutKey, content layout.Content) error {

	glob := filepath.ToSlash(filepath.Join(base, content.Source))
	matches, err := si.fs.Source().Glob(glob)
	if err != nil {
		return err
	}

	for _, rel := range matches {
		entry := &BuildEntry{
			ID:         rel,
			Type:       ContentType,
			SourceRoot: SourceTypeSource,
			SourcePath: rel,

			LayoutKey: key,
			Pipeline:  content.Pipe,
			TagName:   content.Name,
			BaseDir:   base,
		}

		entryNode := NewEntryNode(entry)
		graph.AddNode(entryNode)

		si.buildEntryEdge(graph, entryNode, key, content.Pipe)
	}

	for _, asset := range content.Assets() {
		glob := filepath.ToSlash(filepath.Join(base, asset.Source))
		matches, err := si.fs.Source().Glob(glob)
		if err != nil {
			return err
		}
		for _, rel := range matches {
			entry := &BuildEntry{
				ID:         rel,
				Type:       AssetType,
				SourceRoot: SourceTypeSource,
				SourcePath: rel,

				LayoutKey: key,
				Pipeline:  asset.Pipe,
				TagName:   asset.Name,
				BaseDir:   filepath.Dir(rel),
			}

			entryNode := NewEntryNode(entry)
			graph.AddNode(entryNode)

			si.buildEntryEdge(graph, entryNode, key, asset.Pipe)
		}
	}

	return nil
}

func (si *SourceIndex) scanAssetPattern(ctx context.Context, graph DependencyGraph, base string, key LayoutKey, asset layout.Asset) error {

	glob := filepath.ToSlash(filepath.Join(base, asset.Source))
	matches, err := si.fs.Scheme().Glob(glob)
	if err != nil {
		return err
	}
	for _, rel := range matches {
		entry := &BuildEntry{
			ID:         fmt.Sprintf("asset:scheme:%s", rel),
			Type:       AssetType,
			SourceRoot: SourceTypeScheme,
			SourcePath: rel,

			LayoutKey: key,
			Pipeline:  asset.Pipe,
			TagName:   asset.Name,
			BaseDir:   filepath.Dir(rel),
		}

		entryNode := NewEntryNode(entry)
		graph.AddNode(entryNode)

		si.buildEntryEdge(graph, entryNode, key, asset.Pipe)
	}

	return nil
}

func (si *SourceIndex) buildEntryEdge(graph DependencyGraph, entryNode *EntryNode, lytKey LayoutKey, pipe string) {

	// TODO handle when pipe is native
	if pipe != "" {
		pipeNode := NewPipelineNode(pipe)
		graph.AddNode(pipeNode)
		graph.AddEdge(entryNode, pipeNode)
	}

	lytNode := NewLayoutNode(lytKey)
	graph.AddNode(lytNode)
	graph.AddEdge(entryNode, lytNode)
}

func (si *SourceIndex) IgnoreFile(name string) bool {
	name = filepath.ToSlash(filepath.Clean(name))

	for _, raw := range si.ignore {
		pat := filepath.ToSlash(strings.TrimSpace(raw))
		if pat == "" {
			continue
		}

		if strings.HasSuffix(pat, "/**") {
			prefix := strings.TrimSuffix(pat, "/**")
			if name == prefix || strings.HasPrefix(name, prefix+"/") {
				log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by prefix/**")
				return true
			}
			continue
		}
		if !strings.ContainsAny(pat, "*?[]") {
			if name == pat || strings.HasPrefix(name, pat+"/") {
				log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by exact/prefix")
				return true
			}
			continue
		}

		ok, err := path.Match(pat, name)
		if err != nil {
			continue
		}
		if ok {
			log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by glob")
			return true
		}
	}
	return false
}
