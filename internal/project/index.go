package project

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

	"craeft/internal/layout"
	"craeft/internal/log"
)

type sourceTree interface {
	FS() fs.FS
	Glob(pattern string) ([]string, error)
}

type schemeTree interface {
	FindFile(targetPath, pattern string) ([]string, error)
	Glob(pattern string) ([]string, error)
}

type layoutFinder interface {
	Parse(key LayoutKey) (*layout.Layout, error)
	LayoutsForDir(ctx context.Context, path string) (map[string]LayoutKey, error)
	LayoutsInDir(ctx context.Context, path string) (map[string]LayoutKey, error)
}

type graphBuilder interface {
	AddNode(node Node)
	AddEdge(dependent Node, dependency Node)
}

type SourceIndex struct {
	sourceFS sourceTree
	schemeFS schemeTree

	layouts layoutFinder

	ignore *IgnoreList
}

func NewSourceIndex(sourceFs sourceTree, schemeFS schemeTree, lc layoutFinder, ignore *IgnoreList) *SourceIndex {
	return &SourceIndex{
		sourceFS: sourceFs,
		schemeFS: schemeFS,
		layouts:  lc,

		ignore: ignore,
	}
}

// Scan performs a full scan of the entire source tree.
// Used during initial build.
func (si *SourceIndex) Scan(ctx context.Context, graph graphBuilder) error {
	seenAssets := make(map[string]struct{})
	return fs.WalkDir(si.sourceFS.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
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
		if si.ignore.Match(path) {
			return fs.SkipDir
		}

		base := filepath.ToSlash(path)
		return si.scanDirectory(ctx, graph, base, seenAssets)
	})
}

// ScanDirectory performs a partial scan of a specific directory.
// Stops at subdirectories that have their own layouts.
// Used in watch mode when a directory is created or layout is added.
func (si *SourceIndex) PartialScan(ctx context.Context, graph graphBuilder, baseDir string) error {
	seenAssets := make(map[string]struct{})
	return fs.WalkDir(si.sourceFS.FS(), baseDir, func(path string, d fs.DirEntry, walkErr error) error {
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
		if si.ignore.Match(path) {
			return fs.SkipDir
		}

		// Stop at subdirs with layouts (partial scan only)
		if path != baseDir {
			lmap, err := si.layouts.LayoutsInDir(ctx, path)
			if len(lmap) > 0 {
				return fs.SkipDir
			}

			if err != nil {
				// TODO skip dir and ignore error?
				return fs.SkipDir
			}
		}

		base := filepath.ToSlash(path)
		return si.scanDirectory(ctx, graph, base, seenAssets)
	})
}

func (si *SourceIndex) scanDirectory(ctx context.Context, graph graphBuilder, path string, seenAssets map[string]struct{}) error {

	lkeys, err := si.layouts.LayoutsForDir(ctx, path)
	if err != nil {
		log.WWarn(log.Plan).Err(err).Str("rel", path).Msg("failed to load layout")
		return nil
	}

	for lytPath, lKey := range lkeys {
		lyt, err := si.layouts.Parse(lKey)
		if err != nil {
			return fmt.Errorf("failed to parse layout %q: %w", lytPath, err)
		}

		lytNode := NewLayoutNode(lKey)
		graph.AddNode(lytNode)

		for _, frag := range lyt.Fragments() {
			fragPaths, err := si.schemeFS.FindFile(path, frag.Source)
			if err != nil {
				return err
			}

			fragNode := NewFragmentNode(fragPaths[0])
			graph.AddNode(fragNode)
			graph.AddEdge(lytNode, fragNode)
		}

		if lyt.TplPath != "" {
			tplPaths, err := si.schemeFS.FindFile(path, lyt.TplPath)
			if err != nil {
				return err
			}

			tplNode := NewTemplateNode(tplPaths[0])
			graph.AddNode(tplNode)
			graph.AddEdge(lytNode, tplNode)
		}

		for _, content := range lyt.Contents() {
			err := si.scanContent(ctx, graph, path, lKey, content)
			if err != nil {
				continue
			}
		}
		if _, done := seenAssets[lKey.LayoutPath]; !done {
			seenAssets[lKey.LayoutPath] = struct{}{}
			for _, asset := range lyt.Assets() {
				err := si.scanAsset(ctx, graph, lKey, asset)
				if err != nil {
					continue
				}
			}
		}

	}

	return nil
}

func (si *SourceIndex) scanContent(ctx context.Context, graph graphBuilder, base string, key LayoutKey, content layout.Content) error {

	contentKey := LayoutKey{
		CtxPath:        key.CtxPath,
		LayoutPath:     key.LayoutPath,
		VarFingerprint: varFingerprint(content.Vars()),
	}

	glob := filepath.ToSlash(filepath.Join(base, content.Source))
	matches, err := si.sourceFS.Glob(glob)
	if err != nil {
		return err
	}

	for _, rel := range matches {
		entry := &BuildEntry{
			ID:         rel + ":" + content.Name,
			Type:       ContentType,
			SourceRoot: SourceTypeSource,
			SourcePath: rel,

			LayoutKey:   contentKey,
			Pipeline:    content.Pipe,
			TagName:     content.Name,
			BaseDir:     base,
			ContentVars: content.Vars(),
		}

		entryNode := NewEntryNode(entry)
		graph.AddNode(entryNode)

		si.buildEntryEdge(graph, entryNode, key, content.Pipe)
	}

	for _, asset := range content.Assets() {
		glob := filepath.ToSlash(filepath.Join(base, asset.Source))
		matches, err := si.sourceFS.Glob(glob)
		if err != nil {
			return err
		}
		for _, rel := range matches {
			entry := &BuildEntry{
				ID:         "asset:source:" + rel,
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

func (si *SourceIndex) scanAsset(ctx context.Context, graph graphBuilder, key LayoutKey, asset layout.Asset) error {

	glob := filepath.ToSlash(asset.Source)
	matches, err := si.schemeFS.Glob(glob)
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

func (si *SourceIndex) buildEntryEdge(graph graphBuilder, entryNode *EntryNode, lytKey LayoutKey, pipe string) {

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

// AddFile adds a single file to the graph by finding which content or asset
// pattern it matches in the layout governing its directory.
//
// This is the targeted alternative to PartialScan for watch-mode created events:
// instead of walking the whole directory and re-evaluating all patterns, we have
// the specific file and just need to find its slot.
//
// Returns the newly created BuildEntry and true if the file matched a pattern,
// or nil and false if it belongs to no pattern (not an error — the file may
// simply not be a tracked source file).
func (si *SourceIndex) AddFile(ctx context.Context, graph graphBuilder, filePath string, sourceRoot SourceType) ([]*BuildEntry, bool) {
	dir := filepath.ToSlash(filepath.Dir(filePath))

	lkeys, err := si.layouts.LayoutsForDir(ctx, dir)
	if err != nil {
		log.WWarn(log.Plan).Err(err).Str("path", filePath).Msg("AddFile: no layout found for directory")
		return nil, false
	}

	for _, lKey := range lkeys {
		lyt, err := si.layouts.Parse(lKey)
		if err != nil {
			log.WWarn(log.Plan).Err(err).Str("layout", lKey.LayoutPath).Msg("AddFile: failed to parse layout")
			continue
		}

		switch sourceRoot {
		case SourceTypeSource:
			if entries := si.matchContent(graph, filePath, dir, lKey, lyt); len(entries) > 0 {
				return entries, true
			}

		case SourceTypeScheme:
			if entry := si.matchAsset(graph, filePath, dir, lKey, lyt); entry != nil {
				return []*BuildEntry{entry}, true
			}
		}
	}

	return nil, false
}

// matchContent checks whether filePath matches any content or content-asset
// pattern in the layout, builds the entry, and wires it into the graph.
func (si *SourceIndex) matchContent(graph graphBuilder, filePath, base string, lKey LayoutKey, lyt *layout.Layout) []*BuildEntry {
	var entries []*BuildEntry

	for _, content := range lyt.Contents() {

		contentKey := LayoutKey{
			CtxPath:        lKey.CtxPath,
			LayoutPath:     lKey.LayoutPath,
			VarFingerprint: varFingerprint(content.Vars()),
		}

		glob := filepath.ToSlash(filepath.Join(base, content.Source))

		if matched, err := filepath.Match(glob, filePath); err == nil && matched {
			entry := &BuildEntry{
				ID:          filePath + ":" + content.Name,
				Type:        ContentType,
				SourceRoot:  SourceTypeSource,
				SourcePath:  filePath,
				LayoutKey:   contentKey,
				Pipeline:    content.Pipe,
				TagName:     content.Name,
				BaseDir:     base,
				ContentVars: content.Vars(),
			}
			entryNode := NewEntryNode(entry)
			graph.AddNode(entryNode)
			si.buildEntryEdge(graph, entryNode, lKey, content.Pipe)
			entries = append(entries, entry)
			continue
		}

		for _, asset := range content.Assets() {
			glob := filepath.ToSlash(filepath.Join(base, asset.Source))

			if matched, err := filepath.Match(glob, filePath); err == nil && matched {
				entry := &BuildEntry{
					ID:         "asset:source:" + filePath,
					Type:       AssetType,
					SourceRoot: SourceTypeSource,
					SourcePath: filePath,
					LayoutKey:  lKey,
					Pipeline:   asset.Pipe,
					TagName:    asset.Name,
					BaseDir:    filepath.Dir(filePath),
				}
				entryNode := NewEntryNode(entry)
				graph.AddNode(entryNode)
				si.buildEntryEdge(graph, entryNode, lKey, asset.Pipe)
				entries = append(entries, entry)
				return entries
			}
		}
	}
	return entries
}

// matchAsset checks whether filePath matches any scheme-level asset
// pattern in the layout, builds the entry, and wires it into the graph.
func (si *SourceIndex) matchAsset(graph graphBuilder, filePath, base string, lKey LayoutKey, lyt *layout.Layout) *BuildEntry {
	for _, asset := range lyt.Assets() {
		glob := filepath.ToSlash(asset.Source)

		if matched, err := filepath.Match(glob, filePath); err == nil && matched {
			entry := &BuildEntry{
				ID:         fmt.Sprintf("asset:scheme:%s", filePath),
				Type:       AssetType,
				SourceRoot: SourceTypeScheme,
				SourcePath: filePath,
				LayoutKey:  lKey,
				Pipeline:   asset.Pipe,
				TagName:    asset.Name,
				BaseDir:    filepath.Dir(filePath),
			}
			entryNode := NewEntryNode(entry)
			graph.AddNode(entryNode)
			si.buildEntryEdge(graph, entryNode, lKey, asset.Pipe)
			return entry
		}
	}

	return nil
}
