package project

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

// TestFS wraps fstest.MapFS to provide os.Root-like functionality
type TestFS struct {
	mapFS fstest.MapFS
	root  string
}

// NewTestFS creates a new in-memory filesystem for testing
func NewTestFS(files map[string]string) *TestFS {
	mapFS := make(fstest.MapFS)
	for path, content := range files {
		mapFS[path] = &fstest.MapFile{
			Data:    []byte(content),
			Mode:    0644,
			ModTime: time.Now(),
		}
	}
	return &TestFS{
		mapFS: mapFS,
		root:  "/test",
	}
}

func (t *TestFS) FS() fs.FS {
	return t.mapFS
}

func (t *TestFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(t.mapFS, name)
}

func (t *TestFS) Close() error {
	return nil
}

type TestProjectBuilder struct {
	t             *testing.T
	sourceFiles   map[string]string
	schemeFiles   map[string]string
	pipelineFiles map[string]map[string]string // pipeline name -> files
	config        *CraeftConfig
}

func NewTestProject(t *testing.T) *TestProjectBuilder {
	return &TestProjectBuilder{
		t:             t,
		sourceFiles:   make(map[string]string),
		schemeFiles:   make(map[string]string),
		pipelineFiles: make(map[string]map[string]string),
		config: &CraeftConfig{
			OutputPath: "/test/dist",
			Layout:     "layout.xml",
			Pipes:      []string{"md"},
			Variables:  make(map[string]any),
		},
	}
}

func (b *TestProjectBuilder) WithSource(path, content string) *TestProjectBuilder {
	b.sourceFiles[path] = content
	return b
}

func (b *TestProjectBuilder) WithScheme(path, content string) *TestProjectBuilder {
	b.schemeFiles[path] = content
	return b
}

func (b *TestProjectBuilder) WithPipeline(name string, files map[string]string) *TestProjectBuilder {
	b.pipelineFiles[name] = files
	return b
}

func (b *TestProjectBuilder) WithConfig(modifier func(*CraeftConfig)) *TestProjectBuilder {
	modifier(b.config)
	return b
}

func (b *TestProjectBuilder) Build() (*Project, func()) {
	tempDir := b.t.TempDir()

	sourceDir := filepath.Join(tempDir, "source")
	schemeDir := filepath.Join(tempDir, "scheme")
	outputDir := filepath.Join(tempDir, "dist")
	pipesDir := filepath.Join(tempDir, "pipes")

	for _, dir := range []string{sourceDir, schemeDir, outputDir, pipesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	for path, content := range b.sourceFiles {
		fullPath := filepath.Join(sourceDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			b.t.Fatalf("Failed to create source dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			b.t.Fatalf("Failed to write source file: %v", err)
		}
	}

	for path, content := range b.schemeFiles {
		fullPath := filepath.Join(schemeDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			b.t.Fatalf("Failed to create scheme dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			b.t.Fatalf("Failed to write scheme file: %v", err)
		}
	}

	for pipeName, files := range b.pipelineFiles {
		pipeDir := filepath.Join(pipesDir, pipeName)
		for path, content := range files {
			fullPath := filepath.Join(pipeDir, path)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				b.t.Fatalf("Failed to create pipeline dir: %v", err)
			}
			if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
				b.t.Fatalf("Failed to write pipeline file: %v", err)
			}
		}

		b.config.Pipes = append(b.config.Pipes, pipeName)
	}

	b.config.SourcePath = sourceDir
	b.config.SchemePath = schemeDir
	b.config.OutputPath = outputDir
	b.config.PipesPath = pipesDir

	proj, err := New(b.config)
	if err != nil {
		b.t.Fatalf("Failed to create project: %v", err)
	}

	cleanup := func() {
		proj.Close()
	}

	return proj, cleanup
}

func BasicLayoutXML() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="layout.html">
	<Content name="content" src="*.md" pipe="md" />
</Layout>`
}

func BasicLayoutHTML() string {
	return `<!DOCTYPE html>
<html>
<head><title>Test</title></head>
<body>{{content}}</body>
</html>`
}

func BasicMarkdownPipeline() map[string]string {
	return map[string]string{
		"config.yaml": `name: md
entry_point: index.js
source_suffix: md
target_suffix: html`,
		"index.js": `
function transform(input) {
	return input.replace(/^# (.+)$/gm, '<h1>$1</h1>');
}
`,
	}
}

// AssertFileExists checks if a file exists in the output directory
func AssertFileExists(t *testing.T, outputPath, relativePath string) {
	t.Helper()
	fullPath := filepath.Join(outputPath, relativePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("Expected file %s to exist, but it doesn't", relativePath)
	}
}

// AssertFileContent checks if a file has expected content
func AssertFileContent(t *testing.T, outputPath, relativePath, expected string) {
	t.Helper()
	fullPath := filepath.Join(outputPath, relativePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", relativePath, err)
	}
	if string(content) != expected {
		t.Errorf("File %s content mismatch.\nExpected:\n%s\nGot:\n%s",
			relativePath, expected, string(content))
	}
}

// AssertNoError is a helper to fail tests on unexpected errors
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}
