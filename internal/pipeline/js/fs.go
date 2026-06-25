package js

import (
	"craeft/internal/utils"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dop251/goja"
)

type FsApi struct {
	root *utils.RootFS
}

func NewFsApi(root *utils.RootFS) *FsApi {
	return &FsApi{root: root}
}

// @ts namespace fs
//
// @doc
// File system utilities.
func (fs *FsApi) GetModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()

	module.Set("bytes", func(call goja.FunctionCall) goja.Value { return fs.bytes(vm, call) })
	module.Set("text", func(call goja.FunctionCall) goja.Value { return fs.text(vm, call) })
	module.Set("json", func(call goja.FunctionCall) goja.Value { return fs.readJSON(vm, call) })
	module.Set("exists", func(call goja.FunctionCall) goja.Value { return fs.exists(vm, call) })
	module.Set("isFile", func(call goja.FunctionCall) goja.Value { return fs.isFile(vm, call) })
	module.Set("isDirectory", func(call goja.FunctionCall) goja.Value { return fs.isDirectory(vm, call) })
	module.Set("list", func(call goja.FunctionCall) goja.Value { return fs.list(vm, call) })
	module.Set("walk", func(call goja.FunctionCall) goja.Value { return fs.walk(vm, call) })
	module.Set("size", func(call goja.FunctionCall) goja.Value { return fs.size(vm, call) })
	module.Set("lastModified", func(call goja.FunctionCall) goja.Value { return fs.lastModified(vm, call) })

	return module
}

// @ts function fs.bytes
//
// @doc
// Reads a file and returns its contents as a Uint8Array.
// Returns undefined if the file does not exist.
// Throws an Error for all other I/O failures.
//
// @param path - string - Path relative to the fs root.
// @returns Uint8Array | undefined
func (f *FsApi) bytes(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	path := mustStringArg(vm, "fs.bytes", call.Argument(0))
	data, err := f.root.ReadFile(path)
	if isNotExist(err) {
		return goja.Undefined()
	}
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.bytes: %w", err)))
	}
	buf := vm.NewArrayBuffer(data)
	u8, err := vm.New(vm.Get("Uint8Array").ToObject(vm), vm.ToValue(buf))
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.bytes: construct Uint8Array: %w", err)))
	}
	return u8
}

// @ts function fs.text
//
// @doc
// Reads a file and returns its contents as a UTF-8 string.
// Returns undefined if the file does not exist.
// Throws an Error for all other I/O failures.
//
// @param path - string - Path relative to the fs root.
// @returns string | undefined
func (f *FsApi) text(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	path := mustStringArg(vm, "fs.text", call.Argument(0))
	data, err := f.root.ReadFile(path)
	if isNotExist(err) {
		return goja.Undefined()
	}
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.text: %w", err)))
	}
	return vm.ToValue(string(data))
}

// @ts function fs.json
//
// @doc
// Reads and parses a JSON file, returning the parsed value.
// Returns undefined if the file does not exist.
// Throws a SyntaxError if the file content is not valid JSON.
// Throws an Error for all other I/O failures.
//
// @param path - string - Path relative to the fs root.
// @returns any | undefined
func (f *FsApi) readJSON(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	path := mustStringArg(vm, "fs.json", call.Argument(0))
	data, err := f.root.ReadFile(path)
	if isNotExist(err) {
		return goja.Undefined()
	}
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.json: %w", err)))
	}
	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.json: %s: %w", path, err)))
	}
	return vm.ToValue(result)
}

// @ts function fs.exists
//
// @doc
// Reports whether a path exists (as a file, directory, or any other entry).
//
// @param path - string - Path relative to the fs root.
// @returns boolean
func (f *FsApi) exists(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	path := mustStringArg(vm, "fs.exists", call.Argument(0))
	_, err := f.root.Stat(path)
	return vm.ToValue(!isNotExist(err))
}

// @ts function fs.isFile
//
// @doc
// Reports whether the path refers to a regular file.
// Returns false if the path does not exist or is not a file.
//
// @param path - string - Path relative to the fs root.
// @returns boolean
func (f *FsApi) isFile(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	path := mustStringArg(vm, "fs.isFile", call.Argument(0))
	info, err := f.root.Stat(path)
	return vm.ToValue(err == nil && info.Mode().IsRegular())
}

// @ts function fs.isDirectory
//
// @doc
// Reports whether the path refers to a directory.
// Returns false if the path does not exist or is not a directory.
//
// @param path - string - Path relative to the fs root.
// @returns boolean
func (f *FsApi) isDirectory(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	path := mustStringArg(vm, "fs.isDirectory", call.Argument(0))
	info, err := f.root.Stat(path)
	return vm.ToValue(err == nil && info.IsDir())
}

// @ts function fs.size
//
// @doc
// Returns the size of a file in bytes.
// Returns undefined if the file does not exist.
// Throws an Error for all other I/O failures.
//
// @param path - string - Path relative to the fs root.
// @returns number | undefined
func (f *FsApi) size(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	path := mustStringArg(vm, "fs.size", call.Argument(0))
	info, err := f.root.Stat(path)
	if isNotExist(err) {
		return goja.Undefined()
	}
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.size: %w", err)))
	}
	return vm.ToValue(info.Size())
}

// @ts function fs.lastModified
//
// @doc
// Returns the last modification time of a file as a JavaScript Date object.
// Returns undefined if the file does not exist.
// Throws an Error for all other I/O failures.
//
// @param path - string - Path relative to the fs root.
// @returns Date | undefined
func (f *FsApi) lastModified(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	path := mustStringArg(vm, "fs.lastModified", call.Argument(0))
	info, err := f.root.Stat(path)
	if isNotExist(err) {
		return goja.Undefined()
	}
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.lastModified: %w", err)))
	}
	return newJSDate(vm, info.ModTime())
}

// @ts interface fs.FileEntry
//
// @doc
// Represents a single filesystem entry as returned by fs.list and fs.walk.
// The path field is always slash-separated and prefixed with /, relative to the fs root.
//
// @field name - string - The entry's filename without any directory component.
// @field path - string - Full path relative to the fs root, slash-separated, with leading slash.
// @field size - number - Size in bytes. Always 0 for directories.
// @field modTime - Date - Last modification time as a JavaScript Date.
// @field isFile - boolean - True if the entry is a regular file.
// @field isDirectory - boolean - True if the entry is a directory.
// @field isSymlink - boolean - True if the entry is a symbolic link.
func fileEntry(vm *goja.Runtime, dir, name string, info os.FileInfo) map[string]any {
	return map[string]any{
		"name":        name,
		"path":        filepath.ToSlash(filepath.Join(dir, name)),
		"size":        info.Size(),
		"modTime":     newJSDate(vm, info.ModTime()),
		"isFile":      info.Mode().IsRegular(),
		"isDirectory": info.IsDir(),
		"isSymlink":   info.Mode()&os.ModeSymlink != 0,
	}
}

// @ts function fs.list
//
// @doc
// Returns the direct children of a directory as an array of FileEntry objects.
// The directory itself is not included. Entries are in filesystem order (typically alphabetical).
// Does not recurse into subdirectories — use fs.walk for recursive traversal.
// Throws an Error if the path does not exist or is not a directory.
//
// @param path - string - Path relative to the fs root.
// @returns FileEntry[]
func (f *FsApi) list(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	path := mustStringArg(vm, "fs.list", call.Argument(0))

	entries, err := fs.ReadDir(f.root.FS(), cleanPath(path))
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.list: %w", err)))
	}

	result := make([]any, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue // entry disappeared between ReadDir and Info — skip silently
		}
		result = append(result, fileEntry(vm, path, e.Name(), info))
	}
	return vm.ToValue(result)
}

// @ts function fs.walk
//
// @doc
// Recursively traverses a directory and returns all entries (files and subdirectories)
// as a flat array of FileEntry objects. The root path itself is not included.
// Entries are visited in lexical order. Symlinks are not followed.
// Throws an Error if the root path does not exist or is not a directory.
//
// @param path - string - Path relative to the fs root.
// @returns FileEntry[]
func (f *FsApi) walk(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	root := mustStringArg(vm, "fs.walk", call.Argument(0))
	clean := cleanPath(root)

	var result []any
	err := fs.WalkDir(f.root.FS(), clean, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == clean {
			return nil // skip unreadable entries; skip the root itself
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		result = append(result, fileEntry(vm, dir, d.Name(), info))
		return nil
	})
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.walk: %w", err)))
	}
	return vm.ToValue(result)
}
