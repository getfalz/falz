package server

import (
	"craeft/internal/project"
	"path/filepath"

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
}

// NewFileClassifier creates a new file classifier
func NewFileClassifier(p *project.Project) *FileClassifier {
	return &FileClassifier{project: p}
}

// Classify determines the file type and modification type for a given path and event
func (fc *FileClassifier) Classify(path string, event notify.Event, root string) FileClassification {
	// Determine modification type
	modType := ModTypeModified
	switch event {
	case notify.Create:
		modType = ModTypeCreated
	case notify.Remove:
		modType = ModTypeDeleted
	case notify.Write:
		modType = ModTypeModified
	}

	var fileType FileType

	switch root {
	case "source":
		fileType = fc.classifySourceFile(path)
	case "scheme":
		fileType = fc.classifySchemeFile(path)
	case "pipes":
		fileType = FileTypePipeline
	default:
		fileType = FileTypeUnknown
	}

	return FileClassification{
		Type: fileType,
		Mod:  modType,
		Path: path,
	}
}

// classifySourceFile determines if a source file is content or asset
func (fc *FileClassifier) classifySourceFile(path string) FileType {
	dir := filepath.Dir(path)

	lmap, err := fc.project.GetLayouts(dir)
	if err != nil {
		return FileTypeUnknown
	}

	for _, lyt := range lmap {
		for _, cc := range lyt.Contents() {
			glob := filepath.ToSlash(filepath.Join(dir, cc.Source))
			matched, err := filepath.Match(glob, path)
			if err == nil && matched {
				return FileTypeContent
			}

			for _, a := range cc.Assets() {
				glob := filepath.ToSlash(filepath.Join(dir, a.Source))
				matched, err := filepath.Match(glob, path)
				if err == nil && matched {
					return FileTypeContentAsset
				}
			}
		}
	}

	return FileTypeUnknown
}

// classifySchemeFile determines if a scheme file is layout, fragment, or asset
func (fc *FileClassifier) classifySchemeFile(path string) FileType {
	if ok, err := filepath.Match(fc.project.Config.Layout, path); err == nil && ok {
		return FileTypeLayout
	}

	if fc.project.Layouts.HasDep(path) {
		// fmt.Printf("===Matched Layout.HasDep fragment: %s\n", path)
		return FileTypeFragment
	}

	dir := filepath.Dir(path)
	lmap, err := fc.project.GetLayouts(dir)
	if err != nil {
		return FileTypeUnknown
	}

	for _, lyt := range lmap {

		for _, f := range lyt.Fragments() {
			// TODO: DEBUG
			// fmt.Printf("===Fragments: %s = %s\n", path, f.Source)
			if path == f.Source {
				// fmt.Printf("===Matched Fragment: %s\n", path)
				return FileTypeFragment
			}
		}

		for _, a := range lyt.Assets() {
			glob := filepath.ToSlash(filepath.Join(filepath.Dir(lyt.Path()), a.Source))
			// TODO: DEBUG
			// fmt.Printf("===Asset: glob %q, path %q\n", glob, path)
			matched, err := filepath.Match(glob, path)
			if err == nil && matched {
				// fmt.Printf("===Matched Asset: %s\n", path)
				return FileTypeSchemeAsset
			}
		}
	}

	return FileTypeUnknown
}
