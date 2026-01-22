package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"craeft/internal/project"
)

// Test helpers for handlers

func assertEntryDirty(t *testing.T, plan *project.Plan, id string, shouldBeDirty bool) {
	t.Helper()

	keys := plan.EntryKeys(func(e *project.Entry) bool {
		return e.ID == id && e.Dirty == shouldBeDirty
	})

	if len(keys) == 0 {
		// Entry doesn't match - find out why
		allKeys := plan.EntryKeys(func(e *project.Entry) bool {
			return e.ID == id
		})

		if len(allKeys) == 0 {
			t.Fatalf("Entry %s not found in plan", id)
		}

		// Entry exists but has wrong dirty state
		t.Errorf("Entry %s: expected Dirty=%v, but got opposite", id, shouldBeDirty)
	}
}

func assertEntryExists(t *testing.T, plan *project.Plan, id string, shouldExist bool) {
	t.Helper()
	exists := plan.HasEntry(id)
	if exists != shouldExist {
		t.Errorf("Entry %s: expected exists=%v, got %v", id, shouldExist, exists)
	}
}

func countDirtyEntries(t *testing.T, plan *project.Plan) int {
	t.Helper()
	dirtyEntries := plan.GetDirtyEntries()
	return len(dirtyEntries)
}

// Content Handler Tests

func TestHandleContentModified_BasicChange(t *testing.T) {
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
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	_, err = proj.BuildFromPlan(ctx, plan)
	if err != nil {
		t.Fatalf("Initial build failed: %v", err)
	}

	assertEntryDirty(t, plan, "index.md", false)

	err = os.WriteFile(
		filepath.Join(proj.Config.SourcePath, "index.md"),
		[]byte("# Modified Content"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	err = server.handleContentModified(ctx, plan, []string{"index.md"})
	if err != nil {
		t.Fatalf("handleContentModified failed: %v", err)
	}

	assertEntryDirty(t, plan, "index.md", true)
}

func TestHandleContentModified_MultipleFiles(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("a.md", "# A").
		WithSource("b.md", "# B").
		WithSource("c.md", "# C").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	for _, name := range []string{"a.md", "b.md", "c.md"} {
		err := os.WriteFile(
			filepath.Join(proj.Config.SourcePath, name),
			[]byte("# Modified "+name),
			0644,
		)
		if err != nil {
			t.Fatalf("Failed to modify %s: %v", name, err)
		}
	}

	err = server.handleContentModified(ctx, plan, []string{"a.md", "b.md", "c.md"})
	if err != nil {
		t.Fatalf("handleContentModified failed: %v", err)
	}

	for _, name := range []string{"a.md", "b.md", "c.md"} {
		assertEntryDirty(t, plan, name, true)
	}
}

func TestHandleContentModified_FileNotInPlan(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	// Try to handle a file that doesn't exist in the plan
	err = server.handleContentModified(ctx, plan, []string{"nonexistent.md"})
	if err != nil {
		t.Errorf("Should not error for file not in plan: %v", err)
	}
}

func TestHandleContentCreated_NewFile(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("existing.md", "# Existing").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	// Entry should not exist yet
	assertEntryExists(t, plan, "new.md", false)

	// Create the new file
	err = os.WriteFile(
		filepath.Join(proj.Config.SourcePath, "new.md"),
		[]byte("# New File"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Handle creation
	err = server.handleContentCreated(ctx, plan, []string{"new.md"})
	if err != nil {
		t.Fatalf("handleContentCreated failed: %v", err)
	}

	// Entry should now exist and be dirty
	assertEntryExists(t, plan, "new.md", true)
	assertEntryDirty(t, plan, "new.md", true)
}

func TestHandleContentCreated_MultipleNewFiles(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("existing.md", "# Existing").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	newFiles := []string{"new1.md", "new2.md", "new3.md"}

	// Create new files
	for _, name := range newFiles {
		err := os.WriteFile(
			filepath.Join(proj.Config.SourcePath, name),
			[]byte("# "+name),
			0644,
		)
		if err != nil {
			t.Fatalf("Failed to create %s: %v", name, err)
		}
	}

	// Handle creations
	err = server.handleContentCreated(ctx, plan, newFiles)
	if err != nil {
		t.Fatalf("handleContentCreated failed: %v", err)
	}

	// All should exist and be dirty
	for _, name := range newFiles {
		assertEntryExists(t, plan, name, true)
		assertEntryDirty(t, plan, name, true)
	}
}

func TestHandleContentCreated_AlreadyExists(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("existing.md", "# Existing").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	// Try to create an already existing file
	err = server.handleContentCreated(ctx, plan, []string{"existing.md"})
	if err != nil {
		t.Errorf("Should handle already existing file gracefully: %v", err)
	}
}

func TestHandleContentDeleted_RemoveFile(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("delete-me.md", "# Delete Me").
		WithSource("keep-me.md", "# Keep Me").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	// Both should exist
	assertEntryExists(t, plan, "delete-me.md", true)
	assertEntryExists(t, plan, "keep-me.md", true)

	// Handle deletion
	err = server.handleContentDeleted(ctx, plan, []string{"delete-me.md"})
	if err != nil {
		t.Fatalf("handleContentDeleted failed: %v", err)
	}

	// Deleted file should be gone, other should remain
	assertEntryExists(t, plan, "delete-me.md", false)
	assertEntryExists(t, plan, "keep-me.md", true)
}

func TestHandleContentDeleted_MultipleFiles(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("delete1.md", "# Delete 1").
		WithSource("delete2.md", "# Delete 2").
		WithSource("keep.md", "# Keep").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	// Handle multiple deletions
	err = server.handleContentDeleted(ctx, plan, []string{"delete1.md", "delete2.md"})
	if err != nil {
		t.Fatalf("handleContentDeleted failed: %v", err)
	}

	// Deleted files should be gone
	assertEntryExists(t, plan, "delete1.md", false)
	assertEntryExists(t, plan, "delete2.md", false)
	assertEntryExists(t, plan, "keep.md", true)
}

func TestHandleContentDeleted_NonExistent(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	// Try to delete non-existent file
	err = server.handleContentDeleted(ctx, plan, []string{"nonexistent.md"})
	if err != nil {
		t.Errorf("Should handle non-existent file gracefully: %v", err)
	}
}

// Content Asset Handler Tests

func TestHandleContentAssetModified_BasicChange(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="layout.html">
	<Content name="content" src="*.md" to="{FILENAME}.html" pipe="md">
		<Asset name="images" src="*.png" to="{FILENAME}.png" pipe="copy" />
	</Content>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithSource("image.png", "original-png-data").
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

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	assetID := "asset:source:image.png"
	assertEntryExists(t, plan, assetID, true)

	err = os.WriteFile(
		filepath.Join(proj.Config.SourcePath, "image.png"),
		[]byte("modified-png-data"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify asset: %v", err)
	}

	err = server.handleContentAssetModified(ctx, plan, []string{"image.png"})
	if err != nil {
		t.Fatalf("handleContentAssetModified failed: %v", err)
	}

	assertEntryDirty(t, plan, assetID, true)
}

func TestHandleContentAssetCreated_NewAsset(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="layout.html">
	<Content name="content" src="*.md" to="{FILENAME}.html" pipe="md">
		<Asset name="images" src="*.png" to="{FILENAME}.png" pipe="copy" />
	</Content>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
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

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	newAssetID := "asset:source:new-image.png"
	assertEntryExists(t, plan, newAssetID, false)

	// Create new asset
	err = os.WriteFile(
		filepath.Join(proj.Config.SourcePath, "new-image.png"),
		[]byte("new-png-data"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to create asset: %v", err)
	}

	err = server.handleContentAssetCreated(ctx, plan, []string{"new-image.png"})
	if err != nil {
		t.Fatalf("handleContentAssetCreated failed: %v", err)
	}

	assertEntryExists(t, plan, newAssetID, true)
	assertEntryDirty(t, plan, newAssetID, true)
}

func TestHandleContentAssetDeleted_RemoveAsset(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="layout.html">
	<Content name="content" src="*.md" to="{FILENAME}.html" pipe="md">
		<Asset name="images" src="*.png" to="{FILENAME}.png" pipe="copy" />
	</Content>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithSource("delete-me.png", "png-data").
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

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	assetID := "asset:source:delete-me.png"
	assertEntryExists(t, plan, assetID, true)

	err = server.handleContentAssetDeleted(ctx, plan, []string{"delete-me.png"})
	if err != nil {
		t.Fatalf("handleContentAssetDeleted failed: %v", err)
	}

	assertEntryExists(t, plan, assetID, false)
}

// Layout Handler Tests

func TestHandleLayoutModified_InvalidatesCache(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html" />
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "{{header}}{{content}}").
		WithScheme("header.html", "<header>Original</header>").
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

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	layoutKey := project.LayoutKey{
		RelLayoutPath: "layout.xml",
		BaseDir:       ".",
	}

	initialDeps, ok := proj.Layouts.GetDeps(layoutKey)
	if !ok {
		t.Fatal("Expected layout to be cached initially.")
	}

	initialDepCount := len(initialDeps)
	t.Logf("Initial deps: %d - %v", initialDepCount, initialDeps)

	newLayout := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Fragment name="footer" src="footer.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md"  to="{FILENAME}.html"/>
</Layout>`

	err = os.WriteFile(
		filepath.Join(proj.Config.SchemePath, "layout.xml"),
		[]byte(newLayout),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify layout: %v", err)
	}

	err = os.WriteFile(
		filepath.Join(proj.Config.SchemePath, "footer.html"),
		[]byte("<footer>Footer</footer>"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to create footer: %v", err)
	}

	err = server.handleFragmentModified(ctx, plan, []string{"footer.html"})
	if err != nil {
		t.Fatalf("handleFragmentModified failed: %v", err)
	}
	err = server.handleLayoutModified(ctx, plan, []string{"layout.xml"})
	if err != nil {
		t.Fatalf("handleLayoutModified failed: %v", err)
	}

	newDeps, ok := proj.Layouts.GetDeps(layoutKey)
	if !ok {
		t.Log("Layout cache was invalidated (not yet rebuilt)")

		lmap, _ := proj.GetLayouts(".")
		lyt := lmap["layout.xml"]
		_, err = proj.Layouts.GetOrBuild(ctx, proj.SchemeFS, layoutKey, lyt)
		if err != nil {
			t.Fatalf("Failed to rebuild layout: %v", err)
		}

		newDeps, ok = proj.Layouts.GetDeps(layoutKey)
		if !ok {
			t.Fatal("Layout should be cached after GetOrBuild")
		}
	}

	t.Logf("New deps: %d - %v", len(newDeps), newDeps)

	if len(newDeps) <= initialDepCount {
		t.Errorf("Expected more deps after adding footer, got initial=%d, new=%d",
			initialDepCount, len(newDeps))
	}

	hasFooter := false
	for _, dep := range newDeps {
		if strings.Contains(dep, "footer.html") {
			hasFooter = true
			break
		}
	}

	if !hasFooter {
		t.Errorf("Expected footer.html in new deps, got: %v", newDeps)
	}

	entry, ok := plan.Entry("index.md")
	if !ok {
		t.Fatal("Expected to find index.md entry")
	}

	if !entry.Dirty {
		t.Error("Expected entry to be marked dirty after layout modification")
	}
}

func TestHandleLayoutCreated_NewLayoutInSubdir(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("blog/post.md", "# Post").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	newLayout := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="blog-template.html">
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	blogLayoutPath := filepath.Join(proj.Config.SchemePath, "blog", "layout.xml")
	err = os.MkdirAll(filepath.Dir(blogLayoutPath), 0755)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	err = os.WriteFile(blogLayoutPath, []byte(newLayout), 0644)
	if err != nil {
		t.Fatalf("Failed to create layout: %v", err)
	}

	// Create the template file
	blogTemplatePath := filepath.Join(proj.Config.SchemePath, "blog", "blog-template.html")
	err = os.WriteFile(blogTemplatePath, []byte("<html>{{content}}</html>"), 0644)
	if err != nil {
		t.Fatalf("Failed to create template: %v", err)
	}

	err = server.handleLayoutCreated(ctx, plan, []string{"blog/layout.xml"})
	if err != nil {
		t.Fatalf("handleLayoutCreated failed: %v", err)
	}
}

func TestHandleLayoutDeleted_RescansWithParent(t *testing.T) {
	blogLayout := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="blog-template.html">
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("blog/post.md", "# Post").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithScheme("blog/layout.xml", blogLayout).
		WithScheme("blog/blog-template.html", "<html><div class='blog'>{{content}}</div></html>").
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	err = server.handleLayoutDeleted(ctx, plan, []string{"blog/layout.xml"})
	if err != nil {
		t.Fatalf("handleLayoutDeleted failed: %v", err)
	}
}

// Fragment Handler Tests

func TestHandleFragmentModified_MarksDependentContent(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`
	proj, cleanup := project.NewTestProject(t).
		WithSource("a.md", "# A").
		WithSource("b.md", "# B").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "{{header}}{{content}}").
		WithScheme("header.html", "<header>V1</header>").
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: html
target_suffix: html`,
			"index.js": `function transform(input) { return input; }`,
		}).
		WithConfig(func(c *project.CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	_, err = proj.BuildFromPlan(ctx, plan)
	if err != nil {
		t.Fatalf("Initial build failed: %v", err)
	}

	initialDirty := countDirtyEntries(t, plan)
	if initialDirty != 0 {
		t.Errorf("Expected 0 dirty entries after build, got %d", initialDirty)
	}

	err = os.WriteFile(
		filepath.Join(proj.Config.SchemePath, "header.html"),
		[]byte("<header>V2</header>"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify fragment: %v", err)
	}

	err = server.handleFragmentModified(ctx, plan, []string{"header.html"})
	if err != nil {
		t.Fatalf("handleFragmentModified failed: %v", err)
	}

	finalDirty := countDirtyEntries(t, plan)
	if finalDirty == 0 {
		t.Error("Expected entries to be marked dirty after fragment modification")
	}
	if finalDirty <= initialDirty {
		t.Errorf("Expected more dirty entries after fragment modification, got initial=%d final=%d", initialDirty, finalDirty)
	}
}

func TestHandleFragmentModified_MultipleFragments(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Fragment name="footer" src="footer.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "{{header}}{{content}}{{footer}}").
		WithScheme("header.html", "<header>Header</header>").
		WithScheme("footer.html", "<footer>Footer</footer>").
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

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	err = os.WriteFile(
		filepath.Join(proj.Config.SchemePath, "header.html"),
		[]byte("<header>Modified Header</header>"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify header: %v", err)
	}

	err = os.WriteFile(
		filepath.Join(proj.Config.SchemePath, "footer.html"),
		[]byte("<footer>Modified Footer</footer>"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify footer: %v", err)
	}

	err = server.handleFragmentModified(ctx, plan, []string{"header.html", "footer.html"})
	if err != nil {
		t.Fatalf("handleFragmentModified failed: %v", err)
	}

	assertEntryDirty(t, plan, "index.md", true)
}

// Scheme Asset Handler Tests

func TestHandleSchemeAssetModified_BasicChange(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Asset name="styles" src="*.css" pipe="copy" to="{FILENAME}.css"/>
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", project.BasicLayoutHTML()).
		WithScheme("style.css", "body { color: red; }").
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: css
target_suffix: css`,
			"index.js": `function transform(input) { return input; }`,
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	assetID := "asset:scheme:style.css"
	assertEntryExists(t, plan, assetID, true)

	// Modify asset
	// time.Sleep(100 * time.Millisecond)
	err = os.WriteFile(
		filepath.Join(proj.Config.SchemePath, "style.css"),
		[]byte("body { color: blue; }"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify asset: %v", err)
	}

	err = server.handleSchemeAssetModified(ctx, plan, []string{"style.css"})
	if err != nil {
		t.Fatalf("handleSchemeAssetModified failed: %v", err)
	}

	assertEntryDirty(t, plan, assetID, true)
}

func TestHandleSchemeAssetCreated_NewAsset(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Asset name="styles" src="*.css" pipe="copy" to="{FILENAME}.css"/>
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: css
target_suffix: css`,
			"index.js": `function transform(input) { return input; }`,
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	newAssetID := "asset:scheme:new-style.css"
	assertEntryExists(t, plan, newAssetID, false)

	// Create new asset
	err = os.WriteFile(
		filepath.Join(proj.Config.SchemePath, "new-style.css"),
		[]byte("body { margin: 0; }"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to create asset: %v", err)
	}

	err = server.handleSchemeAssetCreated(ctx, plan, []string{"new-style.css"})
	if err != nil {
		t.Fatalf("handleSchemeAssetCreated failed: %v", err)
	}

	assertEntryExists(t, plan, newAssetID, true)
	assertEntryDirty(t, plan, newAssetID, true)
}

func TestHandleSchemeAssetDeleted_RemoveAsset(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Asset name="styles" src="*.css" pipe="copy" to="{FILENAME}.css"/>
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", project.BasicLayoutHTML()).
		WithScheme("delete-me.css", "body {}").
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: css
target_suffix: css`,
			"index.js": `function transform(input) { return input; }`,
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	assetID := "asset:scheme:delete-me.css"
	assertEntryExists(t, plan, assetID, true)

	err = server.handleSchemeAssetDeleted(ctx, plan, []string{"delete-me.css"})
	if err != nil {
		t.Fatalf("handleSchemeAssetDeleted failed: %v", err)
	}

	assertEntryExists(t, plan, assetID, false)
}

// Pipeline Handler Tests

func TestHandlePipelineModified_ReloadsAndMarksDirty(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout tpl="{{content}}">
	<Content name="content" src="*.md" pipe="copy" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "test").
		WithScheme("layout.xml", layoutXML).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: md
target_suffix: html`,
			"index.js": `function transform(input) { console.log('hello'); return input; }`,
		}).
		WithConfig(func(c *project.CraeftConfig) {
			c.Build.Concurrency = 1
			c.Build.UseCAS = false
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	_, err = proj.BuildFromPlan(ctx, plan)
	if err != nil {
		t.Fatalf("Initial build failed: %v", err)
	}

	initialDirty := countDirtyEntries(t, plan)
	if initialDirty != 0 {
		t.Errorf("Expected 0 dirty entries after build, got %d", initialDirty)
	}

	newPipeline := `
function transform(input) {
	console.log("new", input);
	return "modified " + input;
}
`
	err = os.WriteFile(
		filepath.Join(proj.Config.PipesPath, "copy", "index.js"),
		[]byte(newPipeline),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify pipeline: %v", err)
	}

	err = server.handlePipelineModified(ctx, plan, []string{"copy/index.js"})
	if err != nil {
		t.Fatalf("handlePipelineModified failed: %v", err)
	}

	finalDirty := countDirtyEntries(t, plan)
	if finalDirty == 0 {
		t.Error("Expected entries to be marked dirty after pipeline modification")
	}
	if finalDirty <= initialDirty {
		t.Errorf("Expected more dirty entries after pipeline modification, got initial=%d final=%d", initialDirty, finalDirty)
	}

	_, err = server.project.BuildFromPlan(ctx, plan)
	if err != nil {
		t.Fatalf("second build failed: %v", err)
	}
	content2, _ := os.ReadFile(filepath.Join(proj.Config.OutputPath, "index.html"))
	if !strings.Contains(string(content2), "modified") {
		t.Error("Expected modified in content after pipeline change")
	}
}

func TestHandlePipelineModified_MultiplePipelines(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test MD").
		WithSource("style.css", "body {}").
		WithScheme("layout.xml", `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="layout.html">
	<Asset name="styles" src="*.css" pipe="copy" to="{FILENAME}.{EXT}"/>
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: css
target_suffix: css`,
			"index.js": `function transform(input) { return input; }`,
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	// Modify both pipelines
	// time.Sleep(100 * time.Millisecond)
	err = os.WriteFile(
		filepath.Join(proj.Config.PipesPath, "md", "index.js"),
		[]byte(`function transform(input) { return input.replace(/^# (.+)$/gm, '<h1>MOD: $1</h1>'); }`),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify md pipeline: %v", err)
	}

	err = os.WriteFile(
		filepath.Join(proj.Config.PipesPath, "copy", "index.js"),
		[]byte(`function transform(input) { return "/* modified */ " + input; }`),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify copy pipeline: %v", err)
	}

	err = server.handlePipelineModified(ctx, plan, []string{"md/index.js", "copy/index.js"})
	if err != nil {
		t.Fatalf("handlePipelineModified failed: %v", err)
	}

	// Both content and asset entries should be marked dirty
	contentDirty := plan.EntryKeys(func(e *project.Entry) bool {
		return e.Type == "content" && e.Dirty && e.Pipeline == "md"
	})

	if len(contentDirty) == 0 {
		t.Error("Expected content entries to be marked dirty")
	}
}

// Integration Tests

func TestHandlers_CompleteWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html">
		<Asset name="images" src="*.png" pipe="copyimg" to="{FILENAME}.{EXT}"/>
	</Content>
</Layout>`

	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithSource("image.png", "png-data").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "{{header}}{{content}}").
		WithScheme("header.html", "<header>Header</header>").
		WithPipeline("md", project.BasicMarkdownPipeline()).
		WithPipeline("copyimg", map[string]string{
			"config.yaml": `name: copyimg
entry_point: index.js
source_suffix: "png"
target_suffix: "png.out"`,
			"index.js": `function transform(input) { return input; }`,
		}).
		WithPipeline("copy", map[string]string{
			"config.yaml": `name: copy
entry_point: index.js
source_suffix: "md"
target_suffix: "html"`,
			"index.js": `function transform(input) { return input; }`,
		}).
		WithConfig(func(c *project.CraeftConfig) {
			c.Build.Concurrency = 1
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	_, err = proj.BuildFromPlan(ctx, plan)
	if err != nil {
		t.Fatalf("Initial build failed: %v", err)
	}

	// Test 1: Modify content
	// time.Sleep(100 * time.Millisecond)
	err = os.WriteFile(
		filepath.Join(proj.Config.SourcePath, "index.md"),
		[]byte("# Modified Content"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify content: %v", err)
	}

	err = server.handleContentModified(ctx, plan, []string{"index.md"})
	if err != nil {
		t.Fatalf("handleContentModified failed: %v", err)
	}

	assertEntryDirty(t, plan, "index.md", true)

	// Test 2: Modify fragment
	// time.Sleep(100 * time.Millisecond)
	err = os.WriteFile(
		filepath.Join(proj.Config.SchemePath, "header.html"),
		[]byte("<header>Modified Header</header>"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to modify fragment: %v", err)
	}

	err = server.handleFragmentModified(ctx, plan, []string{"header.html"})
	if err != nil {
		t.Fatalf("handleFragmentModified failed: %v", err)
	}

	// Test 3: Create new content
	err = os.WriteFile(
		filepath.Join(proj.Config.SourcePath, "new.md"),
		[]byte("# New Content"),
		0644,
	)
	if err != nil {
		t.Fatalf("Failed to create new content: %v", err)
	}

	err = server.handleContentCreated(ctx, plan, []string{"new.md"})
	if err != nil {
		t.Fatalf("handleContentCreated failed: %v", err)
	}

	assertEntryExists(t, plan, "new.md", true)

	// Test 4: Delete content
	err = server.handleContentDeleted(ctx, plan, []string{"index.md"})
	if err != nil {
		t.Fatalf("handleContentDeleted failed: %v", err)
	}

	assertEntryExists(t, plan, "index.md", false)

	// Verify new.md still exists
	assertEntryExists(t, plan, "new.md", true)
}

func TestHandlers_ConcurrentModifications(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	proj := project.NewTestProject(t).
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline())

	// Create many files
	for i := range 20 {
		proj.WithSource(
			fmt.Sprintf("file%d.md", i),
			fmt.Sprintf("# File %d", i),
		)
	}

	p, cleanup := proj.Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := p.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, p)

	paths := make([]string, 20)
	for i := range 20 {
		paths[i] = fmt.Sprintf("file%d.md", i)

		// time.Sleep(100 * time.Millisecond)
		err := os.WriteFile(
			filepath.Join(p.Config.SourcePath, paths[i]),
			[]byte(fmt.Sprintf("# Modified File %d", i)),
			0644,
		)
		if err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}
	}

	err = server.handleContentModified(ctx, plan, paths)
	if err != nil {
		t.Fatalf("handleContentModified failed: %v", err)
	}

	dirtyCount := countDirtyEntries(t, plan)
	if dirtyCount != 20 {
		t.Errorf("Expected 20 dirty entries, got %d", dirtyCount)
	}
}

func TestHandlers_ErrorHandling(t *testing.T) {
	proj, cleanup := project.NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", project.BasicLayoutXML()).
		WithScheme("layout.html", project.BasicLayoutHTML()).
		WithPipeline("md", project.BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	plan, err := proj.Plan(ctx)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	server := newTestServer(t, proj)

	// Test handling non-existent files (should not error)
	tests := []struct {
		name    string
		handler func(context.Context, *project.Plan, []string) error
		paths   []string
	}{
		{
			"content modified",
			server.handleContentModified,
			[]string{"nonexistent.md"},
		},
		{
			"content created",
			server.handleContentCreated,
			[]string{"nonexistent.md"},
		},
		{
			"content deleted",
			server.handleContentDeleted,
			[]string{"nonexistent.md"},
		},
		{
			"asset deleted",
			server.handleContentAssetDeleted,
			[]string{"nonexistent.png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.handler(ctx, plan, tt.paths)
			if err != nil {
				t.Errorf("Handler should handle non-existent files gracefully: %v", err)
			}
		})
	}
}

// NEW
// Fragment change -> should warm deps (accessible immediately) and mark dependent entries dirty (no rescan).
// func TestHandleFragmentModified_WarmsDepsAndMarksDirty(t *testing.T) {
// 	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
// <Layout src="template.html">
// 	<Fragment name="header" src="header.html" pipe="copy" />
// 	<Content name="content" src="*.md" pipe="md" />
// </Layout>`
//
// 	proj, cleanup := project.NewTestProject(t).
// 		WithSource("a.md", "# A").
// 		WithSource("b.md", "# B").
// 		WithScheme("layout.xml", layoutXML).
// 		WithScheme("template.html", "{{header}}{{content}}").
// 		WithScheme("header.html", "<header>V1</header>").
// 		WithPipeline("md", project.BasicMarkdownPipeline()).
// 		WithPipeline("copy", map[string]string{
// 			"config.yaml": `name: copy
// entry_point: index.js
// source_suffix: html
// target_suffix: html`,
// 			"index.js": `function transform(input) { return input; }`,
// 		}).
// 		WithConfig(func(c *project.CraeftConfig) {
// 			c.Build.Concurrency = 1
// 		}).
// 		Build()
// 	defer cleanup()
//
// 	ctx := context.Background()
// 	plan, err := proj.Plan(ctx)
// 	if err != nil {
// 		t.Fatalf("Failed to create plan: %v", err)
// 	}
// 	server := newTestServer(t, proj)
//
// 	// Initial build clears dirty flags
// 	if _, err := proj.BuildFromPlan(ctx, plan); err != nil {
// 		t.Fatalf("initial build failed: %v", err)
// 	}
// 	if got := countDirtyEntries(t, plan); got != 0 {
// 		t.Fatalf("expected 0 dirty after initial build, got %d", got)
// 	}
//
// 	// Sanity: deps available before change
// 	layoutKey := project.LayoutKey{RelLayoutPath: "layout.xml", BaseDir: "."}
// 	if deps, ok := proj.Layouts.GetDeps(layoutKey); !ok || len(deps) != 2 {
// 		t.Fatalf("expected initial deps [template.html header.html]; ok=%v deps=%v", ok, deps)
// 	}
//
// 	// Modify the fragment (render-only change)
// 	if err := os.WriteFile(
// 		filepath.Join(proj.Config.SchemePath, "header.html"),
// 		[]byte("<header>V2</header>"),
// 		0644,
// 	); err != nil {
// 		t.Fatalf("Failed to modify fragment: %v", err)
// 	}
//
// 	// Handler must: invalidate -> warm (deps usable right away) -> mark dirty (no rescan)
// 	if err := server.handleFragmentModified(ctx, plan, []string{"header.html"}); err != nil {
// 		t.Fatalf("handleFragmentModified failed: %v", err)
// 	}
//
// 	// Deps still accessible immediately (warmup happened)
// 	if deps, ok := proj.Layouts.GetDeps(layoutKey); !ok || len(deps) != 2 {
// 		t.Fatalf("deps not accessible after fragment change, ok=%v deps=%v", ok, deps)
// 	}
// 	// Entries are marked dirty (both a.md and b.md)
// 	if got := countDirtyEntries(t, plan); got < 2 {
// 		t.Fatalf("expected >=2 dirty entries after fragment change, got %d", got)
// 	}
// }
//
// // Guardrail: InvalidateLayoutsFromPaths(layout.xml) is a no-op (reverse index maps fragments/assets, not layouts).
// // Ensures handlers for layout events compute layout keys from the plan and call InvalidateLayouts(..., RescanFS:true).
// func TestInvalidateLayoutsFromPaths_IgnoresLayoutFiles(t *testing.T) {
// 	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
// <Layout src="template.html">
// 	<Fragment name="header" src="header.html" pipe="copy" />
// 	<Content name="content" src="*.md" pipe="md" />
// </Layout>`
//
// 	proj, cleanup := project.NewTestProject(t).
// 		WithSource("index.md", "# idx").
// 		WithScheme("layout.xml", layoutXML).
// 		WithScheme("template.html", "{{header}}{{content}}").
// 		WithScheme("header.html", "<header>H</header>").
// 		WithPipeline("md", project.BasicMarkdownPipeline()).
// 		WithPipeline("copy", map[string]string{
// 			"config.yaml": `name: copy
// entry_point: index.js
// source_suffix: html
// target_suffix: html`,
// 			"index.js": `function transform(input) { return input; }`,
// 		}).
// 		Build()
// 	defer cleanup()
//
// 	ctx := context.Background()
// 	plan, err := proj.Plan(ctx)
// 	if err != nil {
// 		t.Fatalf("Failed to create plan: %v", err)
// 	}
// 	// Warm via initial build
// 	if _, err := proj.BuildFromPlan(ctx, plan); err != nil {
// 		t.Fatalf("initial build failed: %v", err)
// 	}
// 	layoutKey := project.LayoutKey{RelLayoutPath: "layout.xml", BaseDir: "."}
// 	before, ok := proj.Layouts.GetDeps(layoutKey)
// 	if !ok || len(before) != 2 {
// 		t.Fatalf("expected initial deps=2, ok=%v deps=%v", ok, before)
// 	}
//
// 	// Using the "paths" helper with a layout file should do nothing (0 affected keys internally).
// 	if err := proj.InvalidateLayoutsFromPaths(ctx, plan, []string{"layout.xml"}, project.LayoutInvalidationOpts{
// 		RescanFS: true,
// 	}); err != nil {
// 		t.Fatalf("InvalidateLayoutsFromPaths(layout.xml): %v", err)
// 	}
// 	after, ok := proj.Layouts.GetDeps(layoutKey)
// 	if !ok || len(after) != len(before) {
// 		t.Fatalf("deps unexpectedly changed, ok=%v before=%v after=%v", ok, before, after)
// 	}
// }
//
// // Content in a subdirectory with NO local layout should use the parent directory layout.
// func TestContentInSubdir_UsesParentLayout_WhenNoLocalLayout(t *testing.T) {
// 	layoutRoot := `<?xml version="1.0"?>
// <Layout>
//   <Template file="template.html"/>
//   <Fragments>
//     <Fragment name="header" src="header.html"/>
//   </Fragments>
//   <Contents>
//     <Content src="*.md" pipe="md"/>
//   </Contents>
// </Layout>`
//
// 	proj, cleanup := project.NewTestProject(t).
// 		WithSource("sub/a.md", "# A").
// 		WithScheme("layout.xml", layoutRoot).
// 		WithScheme("template.html", "PARENT-TPL {{header}} {{.Content}}").
// 		WithScheme("header.html", "PARENT-HEADER").
// 		WithPipeline("md", project.BasicMarkdownPipeline()).
// 		Build()
// 	defer cleanup()
//
// 	ctx := context.Background()
// 	plan, err := proj.Plan(ctx)
// 	if err != nil {
// 		t.Fatalf("plan: %v", err)
// 	}
// 	// Scan root and sub; sub has no local layout file -> inherits parent
// 	if err := proj.PartialScan(ctx, plan, "."); err != nil {
// 		t.Fatalf("partials: %v", err)
// 	}
// 	entry, ok := plan.EntryBySource("sub/a.md")
// 	if !ok {
// 		t.Fatalf("expected plan to contain sub/a.md")
// 	}
// 	// It should be using the parent layout (layout.xml at root), base dir is "sub"
// 	if entry.LayoutKey.RelLayoutPath != "layout.xml" || entry.LayoutKey.BaseDir != "sub" {
// 		t.Fatalf("expected parent layout for sub/a.md; got key=%+v", entry.LayoutKey)
// 	}
// 	// Build and verify output includes the PARENT template marker
// 	if _, err := proj.BuildFromPlan(ctx, plan); err != nil {
// 		t.Fatalf("build: %v", err)
// 	}
// 	out := filepath.Join(proj.Config.OutputPath, "sub", "a.html")
// 	b, _ := os.ReadFile(out)
// 	if !strings.Contains(string(b), "PARENT-TPL") {
// 		t.Fatalf("expected PARENT-TPL marker in output, got: %s", string(b))
// 	}
// }
//
// // A local layout in the subdirectory takes precedence over the parent layout.
// func TestSubdirLayout_TakesPrecedence_OverParent(t *testing.T) {
// 	layoutRoot := `<?xml version="1.0"?>
// <Layout>
//   <Template file="template.html"/>
//   <Fragments>
//     <Fragment name="header" src="header.html"/>
//   </Fragments>
//   <Contents>
//     <Content src="*.md" pipe="md"/>
//   </Contents>
// </Layout>`
// 	layoutSub := `<?xml version="1.0"?>
// <Layout>
//   <Template file="template.html"/>
//   <Fragments>
//     <Fragment name="header" src="header.html"/>
//   </Fragments>
//   <Contents>
//     <Content src="*.md" pipe="md"/>
//   </Contents>
// </Layout>`
//
// 	proj, cleanup := project.NewTestProject(t).
// 		WithSource("sub/a.md", "# A").
// 		// parent layout & fragments
// 		WithScheme("layout.xml", layoutRoot).
// 		WithScheme("template.html", "PARENT-TPL {{header}} {{.Content}}").
// 		WithScheme("header.html", "PARENT-HEADER").
// 		// subdir layout & its own template marker
// 		WithScheme("sub/layout.xml", layoutSub).
// 		WithScheme("sub/template.html", "SUB-TPL {{header}} {{.Content}}").
// 		// (no sub/header.html yet -> uses parent header initially)
// 		WithPipeline("md", project.BasicMarkdownPipeline()).
// 		Build()
// 	defer cleanup()
//
// 	ctx := context.Background()
// 	plan, err := proj.Plan(ctx)
// 	if err != nil {
// 		t.Fatalf("plan: %v", err)
// 	}
// 	if err := proj.PartialScan(ctx, plan, "."); err != nil {
// 		t.Fatalf("scan: %v", err)
// 	}
// 	entry, ok := plan.EntryBySource("sub/a.md")
// 	if !ok {
// 		t.Fatalf("sub/a.md missing in plan")
// 	}
// 	// Should select sub/layout.xml for RelLayoutPath, with BaseDir "sub"
// 	if entry.LayoutKey.RelLayoutPath != "sub/layout.xml" || entry.LayoutKey.BaseDir != "sub" {
// 		t.Fatalf("expected sub layout; got key=%+v", entry.LayoutKey)
// 	}
// 	// Build and verify the SUB template marker is used
// 	if _, err := proj.BuildFromPlan(ctx, plan); err != nil {
// 		t.Fatalf("build: %v", err)
// 	}
// 	out := filepath.Join(proj.Config.OutputPath, "sub", "a.html")
// 	b, _ := os.ReadFile(out)
// 	if !strings.Contains(string(b), "SUB-TPL") {
// 		t.Fatalf("expected SUB-TPL marker in output, got: %s", string(b))
// 	}
// }
//
// // Deleting a subdir layout should make matched content fall back to the parent layout (different template).
// func TestSubdirLayoutDeleted_FallsBackToParent(t *testing.T) {
// 	layoutRoot := `<?xml version="1.0"?>
// <Layout>
//   <Template file="template.html"/>
//   <Fragments>
//     <Fragment name="header" src="header.html"/>
//   </Fragments>
//   <Contents>
//     <Content src="*.md" pipe="md"/>
//   </Contents>
// </Layout>`
// 	layoutSub := `<?xml version="1.0"?>
// <Layout>
//   <Template file="template.html"/>
//   <Fragments>
//     <Fragment name="header" src="header.html"/>
//   </Fragments>
//   <Contents>
//     <Content src="*.md" pipe="md"/>
//   </Contents>
// </Layout>`
//
// 	proj, cleanup := project.NewTestProject(t).
// 		WithSource("sub/a.md", "# A").
// 		// parent layout/template/fragment
// 		WithScheme("layout.xml", layoutRoot).
// 		WithScheme("template.html", "PARENT-TPL {{header}} {{.Content}}").
// 		WithScheme("header.html", "PARENT-HEADER").
// 		// sub layout/template
// 		WithScheme("sub/layout.xml", layoutSub).
// 		WithScheme("sub/template.html", "SUB-TPL {{header}} {{.Content}}").
// 		WithPipeline("md", project.BasicMarkdownPipeline()).
// 		Build()
// 	defer cleanup()
//
// 	ctx := context.Background()
// 	plan, err := proj.Plan(ctx)
// 	if err != nil {
// 		t.Fatalf("plan: %v", err)
// 	}
// 	server := newTestServer(t, proj)
// 	if err := proj.PartialScan(ctx, plan, "."); err != nil {
// 		t.Fatalf("scan: %v", err)
// 	}
// 	if _, err := proj.BuildFromPlan(ctx, plan); err != nil {
// 		t.Fatalf("build: %v", err)
// 	}
// 	// Ensure SUB template used initially
// 	out := filepath.Join(proj.Config.OutputPath, "sub", "a.html")
// 	b, _ := os.ReadFile(out)
// 	if !strings.Contains(string(b), "SUB-TPL") {
// 		t.Fatalf("expected SUB-TPL before deletion, got: %s", string(b))
// 	}
// 	// Delete sub/layout.xml and trigger handler (rescan)
// 	if err := os.Remove(filepath.Join(proj.Config.SchemePath, "sub", "layout.xml")); err != nil {
// 		t.Fatalf("remove sub/layout.xml: %v", err)
// 	}
// 	if err := server.handleLayoutDeleted(ctx, plan, []string{filepath.Join(proj.Config.SchemePath, "sub", "layout.xml")}); err != nil {
// 		t.Fatalf("handleLayoutDeleted: %v", err)
// 	}
// 	if _, err := proj.BuildFromPlan(ctx, plan); err != nil {
// 		t.Fatalf("rebuild: %v", err)
// 	}
// 	// Now content must fall back to the parent layout/template
// 	b, _ = os.ReadFile(out)
// 	if !strings.Contains(string(b), "PARENT-TPL") {
// 		t.Fatalf("expected fallback to PARENT-TPL after sub layout deletion, got: %s", string(b))
// 	}
// 	entry, ok := plan.EntryBySource("sub/a.md")
// 	if !ok {
// 		t.Fatalf("sub/a.md missing after deletion rescan")
// 	}
// 	if entry.LayoutKey.RelLayoutPath != "layout.xml" || entry.LayoutKey.BaseDir != "sub" {
// 		t.Fatalf("expected fallback to parent layout key; got key=%+v", entry.LayoutKey)
// 	}
// }
//
// // Adding a new fragment in subdir should override the same-named fragment previously used from parent.
// func TestSubdirFragmentOverride_TakesEffect(t *testing.T) {
// 	layoutRoot := `<?xml version="1.0"?>
// <Layout>
//   <Template file="template.html"/>
//   <Fragments>
//     <Fragment name="header" src="header.html"/>
//   </Fragments>
//   <Contents>
//     <Content src="*.md" pipe="md"/>
//   </Contents>
// </Layout>`
// 	layoutSub := `<?xml version="1.0"?>
// <Layout>
//   <Template file="template.html"/>
//   <Fragments>
//     <Fragment name="header" src="header.html"/>
//   </Fragments>
//   <Contents>
//     <Content src="*.md" pipe="md"/>
//   </Contents>
// </Layout>`
//
// 	proj, cleanup := project.NewTestProject(t).
// 		WithSource("sub/a.md", "# A").
// 		// parent layout/template/header
// 		WithScheme("layout.xml", layoutRoot).
// 		WithScheme("template.html", "TPL {{header}} {{.Content}}").
// 		WithScheme("header.html", "PARENT-H").
// 		// sub layout/template (initially uses parent header)
// 		WithScheme("sub/layout.xml", layoutSub).
// 		WithScheme("sub/template.html", "SUB {{header}} {{.Content}}").
// 		WithPipeline("md", project.BasicMarkdownPipeline()).
// 		Build()
// 	defer cleanup()
//
// 	ctx := context.Background()
// 	plan, err := proj.Plan(ctx)
// 	if err != nil {
// 		t.Fatalf("plan: %v", err)
// 	}
// 	server := newTestServer(t, proj)
//
// 	if err := proj.PartialScan(ctx, plan, "."); err != nil {
// 		t.Fatalf("scan: %v", err)
// 	}
// 	if _, err := proj.BuildFromPlan(ctx, plan); err != nil {
// 		t.Fatalf("build: %v", err)
// 	}
// 	// Initially render should use parent header
// 	out := filepath.Join(proj.Config.OutputPath, "sub", "a.html")
// 	b, _ := os.ReadFile(out)
// 	if !strings.Contains(string(b), "PARENT-H") {
// 		t.Fatalf("expected parent header initially, got: %s", string(b))
// 	}
// 	// Now create sub/header.html (override) and fire the fragment handler (no rescan)
// 	if err := os.WriteFile(filepath.Join(proj.Config.SchemePath, "sub", "header.html"), []byte("SUB-H"), 0644); err != nil {
// 		t.Fatalf("write sub/header.html: %v", err)
// 	}
// 	if err := server.handleFragmentModified(ctx, plan, []string{filepath.Join(proj.Config.SchemePath, "sub", "header.html")}); err != nil {
// 		t.Fatalf("handleFragmentModified: %v", err)
// 	}
// 	// Rebuild; should now pick SUB-H
// 	if _, err := proj.BuildFromPlan(ctx, plan); err != nil {
// 		t.Fatalf("rebuild: %v", err)
// 	}
// 	b, _ = os.ReadFile(out)
// 	if !strings.Contains(string(b), "SUB-H") {
// 		t.Fatalf("expected override header SUB-H after creating sub/header.html, got: %s", string(b))
// 	}
// }
