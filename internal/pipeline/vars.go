package pipeline

import (
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
)

type FileInfo struct {
	Path string
	Dir  string
	Name string
	Stem string
	Ext  string

	// BaseDir is the layout-scoping directory - the directory from which
	// the layout governing this file was resolved.
	BaseDir string
}

func NewFileInfo(sourcePath, baseDir string) FileInfo {
	sourcePath = filepath.ToSlash(sourcePath)
	baseDir = filepath.ToSlash(baseDir)

	name := filepath.Base(sourcePath)
	extWithDot := filepath.Ext(name)

	return FileInfo{
		Path:    sourcePath,
		Dir:     filepath.ToSlash(filepath.Dir(sourcePath)),
		Name:    name,
		Stem:    strings.TrimSuffix(name, extWithDot),
		Ext:     strings.TrimPrefix(extWithDot, "."),
		BaseDir: baseDir,
	}
}

type Vars map[string]any

func (v Vars) Set(key string, value any) {
	v[key] = value
}

func (v Vars) ToGojaObject(vm *goja.Runtime) *goja.Object {
	obj := vm.NewObject()
	for key, value := range v {
		if nestedVars, ok := value.(Vars); ok {
			value = nestedVars.ToGojaObject(vm)
		}
		obj.Set(key, value)
	}
	return obj
}
