package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"craeft/internal/project"

	"github.com/rjeczalik/notify"
)

func newTestServer(t *testing.T, proj *project.Project) *Server {
	t.Helper()
	s := NewServer(proj)
	return s
}

// FileClassifier Tests

func TestFileClassifier_ClassifySourceContent(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	classifier := NewFileClassifier(proj)

	classification := classifier.Classify("index.md", notify.Write, "source")

	if classification.Type != FileTypeContent {
		t.Errorf("Expected FileTypeContent, got %s", classification.Type)
	}
	if classification.Mod != ModTypeModified {
		t.Errorf("Expected ModTypeModified, got %s", classification.Mod)
	}
}

func TestFileClassifier_ClassifySourceAsset(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="layout.html">
	<Content name="content" src="*.md" pipe="md">
		<Asset name="images" src="*.png" pipe="copy" />
	</Content>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithSource("image.png", "fake-png").
		WithScheme("layout.xml", layoutXML).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: png
target_suffix: png`,
			"index.js": `function transform(input) { return input; }`,
		}).
		Build()
	defer cleanup()

	classifier := NewFileClassifier(proj)

	classification := classifier.Classify("image.png", notify.Write, "source")

	if classification.Type != FileTypeContentAsset {
		t.Errorf("Expected FileTypeContentAsset, got %s", classification.Type)
	}
}

func TestFileClassifier_ClassifyLayout(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	classifier := NewFileClassifier(proj)

	classification := classifier.Classify("layout.xml", notify.Write, "scheme")

	if classification.Type != FileTypeLayout {
		t.Errorf("Expected FileTypeLayout, got %s", classification.Type)
	}
}

func TestFileClassifier_ClassifyFragment(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md" />
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "{{header}}{{content}}").
		WithScheme("header.html", "<header>Header</header>").
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: html
target_suffix: html`,
			"index.js": `function transform(input) { return input; }`,
		}).
		Build()
	defer cleanup()

	classifier := NewFileClassifier(proj)

	classification := classifier.Classify("header.html", notify.Write, "scheme")

	if classification.Type != FileTypeFragment {
		t.Errorf("Expected FileTypeFragment, got %s", classification.Type)
	}
}

func TestFileClassifier_ClassifyPipeline(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	classifier := NewFileClassifier(proj)

	classification := classifier.Classify("md/index.js", notify.Write, "pipes")

	if classification.Type != FileTypePipeline {
		t.Errorf("Expected FileTypePipeline, got %s", classification.Type)
	}
}

func TestFileClassifier_ModificationTypes(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	classifier := NewFileClassifier(proj)

	tests := []struct {
		event    notify.Event
		expected ModType
	}{
		{notify.Create, ModTypeCreated},
		{notify.Write, ModTypeModified},
		{notify.Remove, ModTypeDeleted},
	}

	for _, tt := range tests {
		t.Run(string(tt.expected), func(t *testing.T) {
			classification := classifier.Classify("index.md", tt.event, "source")
			if classification.Mod != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, classification.Mod)
			}
		})
	}
}

// EventRegistry Tests

func TestEventRegistry_RegisterAndDispatch(t *testing.T) {
	registry := NewEventRegistry()

	called := false
	handler := HandlerFunc(func(ctx context.Context, plan *project.Plan, paths []string) error {
		called = true
		return nil
	})

	registry.Register(FileTypeContent, ModTypeModified, handler)

	err := registry.Dispatch(context.Background(), nil, FileTypeContent, ModTypeModified, []string{"test.md"})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !called {
		t.Error("Handler was not called")
	}
}

func TestEventRegistry_NoHandlerRegistered(t *testing.T) {
	registry := NewEventRegistry()

	err := registry.Dispatch(context.Background(), nil, FileTypeContent, ModTypeModified, []string{"test.md"})

	if err != nil {
		t.Errorf("Expected no error when handler not registered, got %v", err)
	}
}

func TestEventRegistry_MultipleHandlers(t *testing.T) {
	registry := NewEventRegistry()

	contentCalled := false
	assetCalled := false

	contentHandler := HandlerFunc(func(ctx context.Context, plan *project.Plan, paths []string) error {
		contentCalled = true
		return nil
	})

	assetHandler := HandlerFunc(func(ctx context.Context, plan *project.Plan, paths []string) error {
		assetCalled = true
		return nil
	})

	registry.Register(FileTypeContent, ModTypeModified, contentHandler)
	registry.Register(FileTypeContentAsset, ModTypeModified, assetHandler)

	registry.Dispatch(context.Background(), nil, FileTypeContent, ModTypeModified, []string{"test.md"})
	registry.Dispatch(context.Background(), nil, FileTypeContentAsset, ModTypeModified, []string{"test.png"})

	if !contentCalled {
		t.Error("Content handler was not called")
	}
	if !assetCalled {
		t.Error("Asset handler was not called")
	}
}

// File Change Coalescing Tests

func TestServer_CoalesceFileEvents(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	server := newTestServer(t, proj)

	// First event should be returned
	change1 := FileChange{
		Path: "test.md",
		Type: FileTypeContent,
		Mod:  ModTypeCreated,
		Time: time.Now(),
	}

	result1 := server.coalesceFileEvents(change1)
	if result1 == nil {
		t.Error("First event should not be suppressed")
	}

	// Immediate second event should be suppressed
	change2 := FileChange{
		Path: "test.md",
		Type: FileTypeContent,
		Mod:  ModTypeModified,
		Time: time.Now(),
	}

	result2 := server.coalesceFileEvents(change2)
	if result2 != nil {
		t.Error("Rapid second event should be suppressed")
	}

	// After window, event should be returned
	time.Sleep(eventMergeWindow + 10*time.Millisecond)
	change3 := FileChange{
		Path: "test.md",
		Type: FileTypeContent,
		Mod:  ModTypeModified,
		Time: time.Now(),
	}

	result3 := server.coalesceFileEvents(change3)
	if result3 == nil {
		t.Error("Event after merge window should not be suppressed")
	}
}

func TestServer_ResolveEventType(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	server := newTestServer(t, proj)

	tests := []struct {
		name     string
		prev     ModType
		curr     ModType
		expected ModType
	}{
		{"delete then create = modify", ModTypeDeleted, ModTypeCreated, ModTypeModified},
		{"create then modify = create", ModTypeCreated, ModTypeModified, ModTypeCreated},
		{"create then delete = delete", ModTypeCreated, ModTypeDeleted, ModTypeDeleted},
		{"modify then modify = modify", ModTypeModified, ModTypeModified, ModTypeModified},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.resolveEventType(tt.prev, tt.curr)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestServer_GroupChanges(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	server := newTestServer(t, proj)

	changes := []FileChange{
		{Path: "a.md", Type: FileTypeContent, Mod: ModTypeModified},
		{Path: "b.md", Type: FileTypeContent, Mod: ModTypeModified},
		{Path: "a.md", Type: FileTypeContent, Mod: ModTypeModified}, // Duplicate
		{Path: "c.png", Type: FileTypeContentAsset, Mod: ModTypeCreated},
	}

	grouped := server.groupChanges(changes)

	contentKey := changeKey{FileTypeContent, ModTypeModified}
	assetKey := changeKey{FileTypeContentAsset, ModTypeCreated}

	if len(grouped[contentKey]) != 2 {
		t.Errorf("Expected 2 unique content changes, got %d", len(grouped[contentKey]))
	}

	if len(grouped[assetKey]) != 1 {
		t.Errorf("Expected 1 asset change, got %d", len(grouped[assetKey]))
	}
}

// Hot Reload Tests

func TestServer_HotReloadSetup(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithConfig(func(c *project.CraeftConfig) {
			c.FsWatcher.Enabled = true
			c.FsWatcher.HotReload = true
		}).
		Build()
	defer cleanup()

	server := newTestServer(t, proj)

	err := server.HotReload()
	if err != nil {
		t.Errorf("HotReload setup failed: %v", err)
	}

	// Check that WebSocket handler is registered
	// This is a basic check - full WebSocket testing would require more setup
}

func TestServer_SendToClient(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	server := newTestServer(t, proj)

	// Test sending with no clients (should not panic)
	msg := HotReloadMsg{Reload: true, URL: ""}
	server.sendToClient(msg)
}

// Integration Tests

func TestServer_ChangeDetectionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Original").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithConfig(func(c *project.CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	_, err := proj.Build(ctx)
	if err != nil {
		t.Fatalf("Initial build failed: %v", err)
	}

	server := newTestServer(t, proj)

	changes := []FileChange{
		{
			Path: "index.md",
			Type: FileTypeContent,
			Mod:  ModTypeModified,
			Time: time.Now(),
		},
	}

	time.Sleep(10 * time.Millisecond)
	err = os.WriteFile(
		filepath.Join(proj.Config.SourcePath, "index.md"),
		[]byte("# Modified"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	err = server.handleChangeBatch(ctx, changes)
	if err != nil {
		t.Errorf("handleChangeBatch failed: %v", err)
	}

	outputPath := filepath.Join(proj.Config.OutputPath, "index.html")
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	if !strings.Contains(string(content), "Modified") {
		t.Error("Output should contain modified content")
	}
}

func TestServer_MultipleFileChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	proj, cleanup := project.NewTestProject(t).
		WithSource("a.md", "# A").
		WithSource("b.md", "# B").
		WithSource("c.md", "# C").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithConfig(func(c *project.CraeftConfig) {
			c.Build.Concurrency = 1
			c.Build.UseCAS = false
		}).
		Build()
	defer cleanup()

	ctx := context.Background()

	_, err := proj.Build(ctx)
	if err != nil {
		t.Fatalf("Initial build failed: %v", err)
	}

	server := newTestServer(t, proj)

	for _, name := range []string{"a.md", "b.md", "c.md"} {
		time.Sleep(10 * time.Millisecond)
		err := os.WriteFile(
			filepath.Join(proj.Config.SourcePath, name),
			[]byte("# Modified "+name),
			0644,
		)
		if err != nil {
			t.Fatalf("Failed to modify %s: %v", name, err)
		}
	}

	changes := []FileChange{
		{Path: "a.md", Type: FileTypeContent, Mod: ModTypeModified},
		{Path: "b.md", Type: FileTypeContent, Mod: ModTypeModified},
		{Path: "c.md", Type: FileTypeContent, Mod: ModTypeModified},
	}

	err = server.handleChangeBatch(ctx, changes)
	if err != nil {
		t.Errorf("handleChangeBatch failed: %v", err)
	}

	for _, name := range []string{"a.html", "b.html", "c.html"} {
		outputPath := filepath.Join(proj.Config.OutputPath, name)
		content, err := os.ReadFile(outputPath)
		if err != nil {
			t.Errorf("Failed to read %s: %v", name, err)
			continue
		}

		if !strings.Contains(string(content), "Modified") {
			t.Errorf("%s should contain modified content", name)
		}
	}
}

func TestServer_RegisterHandlers(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	server := newTestServer(t, proj)

	expectedHandlers := []struct {
		fileType FileType
		modType  ModType
	}{
		{FileTypeContent, ModTypeModified},
		{FileTypeContent, ModTypeCreated},
		{FileTypeContent, ModTypeDeleted},
		{FileTypeContentAsset, ModTypeModified},
		{FileTypeContentAsset, ModTypeCreated},
		{FileTypeContentAsset, ModTypeDeleted},
		{FileTypeLayout, ModTypeModified},
		{FileTypeLayout, ModTypeCreated},
		{FileTypeLayout, ModTypeDeleted},
		{FileTypeFragment, ModTypeModified},
		{FileTypeFragment, ModTypeCreated},
		{FileTypeFragment, ModTypeDeleted},
		{FileTypeSchemeAsset, ModTypeModified},
		{FileTypeSchemeAsset, ModTypeCreated},
		{FileTypeSchemeAsset, ModTypeDeleted},
		{FileTypePipeline, ModTypeModified},
		{FileTypePipeline, ModTypeCreated},
		{FileTypePipeline, ModTypeDeleted},
	}

	for _, expected := range expectedHandlers {
		handlers, ok := server.eventRegistry.handlers[expected.fileType]
		if !ok {
			t.Errorf("No handlers registered for %s", expected.fileType)
			continue
		}

		if _, ok := handlers[expected.modType]; !ok {
			t.Errorf("No handler registered for %s %s", expected.fileType, expected.modType)
		}
	}
}
