package js

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"craeft/internal/utils"
)

func writeRootFile(t *testing.T, root *utils.RootFS, relPath, content string) {
	t.Helper()
	full := filepath.Join(root.Path(), relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func TestFS_Text_ReadsFileContent(t *testing.T) {
	root := newTestRoot(t)
	writeRootFile(t, root, "hello.txt", "hello world")

	vm := newTestVM(t, root, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			return fs.text('hello.txt');
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "hello world" {
		t.Fatalf("got %v, want %q", result.Export(), "hello world")
	}
}

func TestFS_Text_MissingFile_ReturnsUndefined(t *testing.T) {
	root := newTestRoot(t)
	vm := newTestVM(t, root, HTTPPolicy{}, nil)

	result, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			return fs.text('does-not-exist.txt') === undefined;
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToBoolean() != true {
		t.Fatalf("expected fs.text to return undefined for a missing file")
	}
}

func TestFS_Bytes_ReturnsUint8ArrayWithCorrectData(t *testing.T) {
	root := newTestRoot(t)
	writeRootFile(t, root, "data.bin", "abc")

	vm := newTestVM(t, root, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			const u8 = fs.bytes('data.bin');
			if (!(u8 instanceof Uint8Array)) return "not a Uint8Array";
			return Array.from(u8).join(',');
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "97,98,99" {
		t.Fatalf("got %v, want byte values of \"abc\"", result.Export())
	}
}

func TestFS_JSON_ParsesValidJSON(t *testing.T) {
	root := newTestRoot(t)
	writeRootFile(t, root, "data.json", `{"name":"acme","count":3}`)

	vm := newTestVM(t, root, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			const obj = fs.json('data.json');
			return obj.name + ':' + obj.count;
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "acme:3" {
		t.Fatalf("got %v, want %q", result.Export(), "acme:3")
	}
}

func TestFS_JSON_InvalidJSON_Throws(t *testing.T) {
	root := newTestRoot(t)
	writeRootFile(t, root, "bad.json", `{not valid json`)

	vm := newTestVM(t, root, HTTPPolicy{}, nil)
	_, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			return fs.json('bad.json');
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for malformed JSON")
	}
}

func TestFS_Exists_IsFile_IsDirectory(t *testing.T) {
	root := newTestRoot(t)
	writeRootFile(t, root, "dir/file.txt", "x")

	vm := newTestVM(t, root, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			return [
				fs.exists('dir/file.txt'),
				fs.exists('dir'),
				fs.exists('nope'),
				fs.isFile('dir/file.txt'),
				fs.isFile('dir'),
				fs.isDirectory('dir'),
				fs.isDirectory('dir/file.txt'),
			].join(',');
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "true,true,false,true,false,true,false"
	if result.Export() != want {
		t.Fatalf("got %v, want %q", result.Export(), want)
	}
}

func TestFS_Size_ReturnsByteCount(t *testing.T) {
	root := newTestRoot(t)
	writeRootFile(t, root, "file.txt", "hello")

	vm := newTestVM(t, root, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			return fs.size('file.txt');
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToInteger() != 5 {
		t.Fatalf("got %v, want 5", result.Export())
	}
}

func TestFS_LastModified_ReturnsDateInstance(t *testing.T) {
	root := newTestRoot(t)
	writeRootFile(t, root, "file.txt", "x")

	vm := newTestVM(t, root, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			const d = fs.lastModified('file.txt');
			return d instanceof Date && !isNaN(d.getTime());
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToBoolean() != true {
		t.Fatalf("expected fs.lastModified to return a valid Date")
	}
}

// Verifies fs.list returns only direct children (name, isDirectory) and
// does not recurse into subdirectories.
func TestFS_List_ReturnsDirectChildrenOnly(t *testing.T) {
	root := newTestRoot(t)
	writeRootFile(t, root, "dir/a.txt", "a")
	writeRootFile(t, root, "dir/b.txt", "b")
	writeRootFile(t, root, "dir/sub/c.txt", "c")

	vm := newTestVM(t, root, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			const entries = fs.list('dir');
			return entries.map(e => e.name + ':' + e.isDirectory).sort().join(',');
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "a.txt:false,b.txt:false,sub:true"
	if result.Export() != want {
		t.Fatalf("got %v, want %q (fs.list must not recurse)", result.Export(), want)
	}
}

// Verifies fs.walk recurses into subdirectories and that each entry's path
// is slash-separated and rooted with a leading slash.
func TestFS_Walk_ReturnsAllNestedEntriesRecursively(t *testing.T) {
	root := newTestRoot(t)
	writeRootFile(t, root, "dir/a.txt", "a")
	writeRootFile(t, root, "dir/sub/b.txt", "b")

	vm := newTestVM(t, root, HTTPPolicy{}, nil)
	result, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			const entries = fs.walk('dir');
			return entries.map(e => e.path).sort().join(',');
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/dir/a.txt,/dir/sub,/dir/sub/b.txt"
	if result.Export() != want {
		t.Fatalf("got %v, want %q", result.Export(), want)
	}
}

func TestFS_List_NonexistentDir_Throws(t *testing.T) {
	root := newTestRoot(t)
	vm := newTestVM(t, root, HTTPPolicy{}, nil)

	_, err := runJSFunc(t, vm, `
		function run() {
			const fs = require('fs');
			return fs.list('does-not-exist');
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error for a nonexistent directory")
	}
}

// Every fs.* function that takes a `path: string` argument must reject a
// wrong-typed argument with a JS TypeError.
func TestFS_FunctionsRequiringPath_ThrowTypeErrorOnNonStringArg(t *testing.T) {
	fns := []string{"bytes", "text", "json", "exists", "isFile", "isDirectory", "list", "walk", "size", "lastModified"}

	for _, fn := range fns {
		t.Run(fn, func(t *testing.T) {
			root := newTestRoot(t)
			vm := newTestVM(t, root, HTTPPolicy{}, nil)

			_, err := runJSFunc(t, vm, `
				function run() {
					const fs = require('fs');
					return fs.`+fn+`(42);
				}`, "run")
			if err == nil {
				t.Fatalf("fs.%s(42): expected a TypeError, got none", fn)
			}
			if !strings.Contains(err.Error(), "TypeError") {
				t.Fatalf("fs.%s(42): expected a TypeError, got %v", fn, err)
			}
		})
	}
}
