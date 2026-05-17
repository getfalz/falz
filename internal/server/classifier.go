package server

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/project"
	"path/filepath"
	"strings"

	"github.com/rjeczalik/notify"
)

// FileClassification represents the result of classifying a file
type FileClassification struct {
	Type FileType
	Mod  ModType
	Path string
}

// FileClassifier determines the type of a file based on its path and project structure
type FileClassifier struct {
	project *project.Project
	ignore  project.IgnoreList

	sourcePath string
	schemePath string
	pipesPath  string
	outputPath string
}

// NewFileClassifier creates a new file classifier
func NewFileClassifier(p *project.Project, sourcePath, schemePath, pipesPath, outputPath string, ignore project.IgnoreList) *FileClassifier {
	// TODO do we need each path separately?
	return &FileClassifier{
		project:    p,
		ignore:     ignore,
		sourcePath: sourcePath,
		schemePath: schemePath,
		pipesPath:  pipesPath,
		outputPath: outputPath,
	}
}

// Classify determines the file type and modification type for a given path and event
func (fc *FileClassifier) Classify(ctx context.Context, raw RawChange) *FileChange {
	absPath := raw.Path

	relPath, root := fc.resolveRoot(absPath)
	if root == "" {
		return nil
	}

	relPath = filepath.ToSlash(relPath)

	if root == "source" && fc.ignore.Match(relPath) {
		return nil
	}

	fileType, err := fc.classifyPath(ctx, relPath, root)
	if fileType == FileTypeUnknown || err != nil {
		return nil
	}

	return &FileChange{
		Path: relPath,
		Type: fileType,
		Mod:  raw.Mod,
		Time: raw.Time,
	}
}

func (fc *FileClassifier) resolveRoot(absPath string) (relPath, root string) {
	switch {
	case strings.HasPrefix(absPath, fc.schemePath):
		rel, _ := filepath.Rel(fc.schemePath, absPath)
		return rel, "scheme"
	case strings.HasPrefix(absPath, fc.pipesPath):
		rel, _ := filepath.Rel(fc.pipesPath, absPath)
		return rel, "pipes"
	case strings.HasPrefix(absPath, fc.sourcePath):
		rel, _ := filepath.Rel(fc.sourcePath, absPath)
		return rel, "source"
	default:
		return "", ""
	}
}

func (fc *FileClassifier) classifyPath(ctx context.Context, relPath, root string) (FileType, error) {
	switch root {
	case "source":
		return fc.classifySourceFile(ctx, relPath)
	case "scheme":
		return fc.classifySchemeFile(ctx, relPath)
	case "pipes":
		return FileTypePipeline, nil
	default:
		return FileTypeUnknown, nil
	}
}

func resolveMod(event notify.Event) ModType {
	switch event {
	case notify.Create:
		return ModTypeCreated
	case notify.Remove:
		return ModTypeDeleted
	default:
		return ModTypeModified
	}
}

// classifySourceFile determines if a source file is content or asset
func (fc *FileClassifier) classifySourceFile(ctx context.Context, path string) (FileType, error) {
	dir := filepath.Dir(path)

	keys, err := fc.project.Layouts.LayoutsForDir(ctx, dir)
	if err != nil {
		return FileTypeUnknown, err
	}

	for _, key := range keys {
		lyt, err := fc.project.Layouts.Get(ctx, key)
		if err != nil {
			log.WWarn(log.Server).Err(err).Str("layout", key.LayoutPath).Msg("failed to parse layout")
			continue
		}

		for _, cc := range lyt.Layout().Contents() {
			glob := filepath.ToSlash(filepath.Join(dir, cc.Source))
			matched, err := filepath.Match(glob, path)
			if err == nil && matched {
				return FileTypeContent, nil
			}

			for _, a := range cc.Assets() {
				glob := filepath.ToSlash(filepath.Join(dir, a.Source))
				matched, err := filepath.Match(glob, path)
				if err == nil && matched {
					return FileTypeContentAsset, nil
				}
			}
		}
	}

	return FileTypeUnknown, nil
}

// classifySchemeFile determines if a scheme file is layout, fragment, or asset
func (fc *FileClassifier) classifySchemeFile(ctx context.Context, path string) (FileType, error) {
	if ok, err := filepath.Match(fc.project.Config.Layout, path); err == nil && ok {
		return FileTypeLayout, nil
	}

	dir := filepath.Dir(path)

	keys, err := fc.project.Layouts.LayoutsForDir(ctx, dir)
	if err != nil {
		return FileTypeUnknown, err
	}

	for _, key := range keys {
		lyt, err := fc.project.Layouts.Get(ctx, key)
		if err != nil {
			log.WWarn(log.Server).Err(err).Str("layout", key.LayoutPath).Msg("failed to parse layout")
			continue
		}

		for _, f := range lyt.Layout().Fragments() {
			if path == f.Source {
				return FileTypeFragment, nil
			}
		}

		for _, a := range lyt.Layout().Assets() {
			glob := filepath.ToSlash(filepath.Join(dir, a.Source))
			matched, err := filepath.Match(glob, path)
			if err == nil && matched {
				return FileTypeSchemeAsset, nil
			}
		}
	}

	return FileTypeUnknown, nil
}
