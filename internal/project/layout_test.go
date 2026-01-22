package project

import (
	"context"
	"strings"
	"testing"
)

func TestLayout_BasicParsing(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "<html>{{content}}</html>").
		WithPipeline("md", BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	layouts, err := proj.GetLayouts(".")
	if err != nil {
		t.Fatalf("Failed to get layouts: %v", err)
	}

	if len(layouts) == 0 {
		t.Fatal("Expected at least one layout")
	}
}

func TestLayout_WithFragments(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "{{header}}<main>{{content}}</main>").
		WithScheme("header.html", "<header>Site Header</header>").
		WithPipeline("md", BasicMarkdownPipeline()).
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
	// key := LayoutKey{
	// 	RelLayoutPath: "layout.xml",
	// 	BaseDir:       ".",
	// }

	layouts, err := proj.GetLayouts(".")
	if err != nil {
		t.Fatalf("Failed to get layouts: %v", err)
	}

	layout := layouts["layout.xml"]
	if layout == nil {
		t.Fatal("Expected to find layout")
	}

	rendered, deps, err := layout.Render(ctx, ".", proj.SchemeFS)
	if err != nil {
		t.Fatalf("Failed to render layout: %v", err)
	}

	if !strings.Contains(rendered, "Site Header") {
		t.Error("Expected rendered layout to contain fragment")
	}

	if len(deps) == 0 {
		t.Error("Expected dependencies to be tracked")
	}
}

func TestLayout_WithAssets(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Asset name="styles" src="*.css" pipe="copy" to="{FILENAME}.{EXT}"/>
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithSource("index.md", "# Test").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "<html>{{content}}</html>").
		WithScheme("style.css", "body {}").
		WithPipeline("md", BasicMarkdownPipeline()).
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

	contentKeys := plan.EntryKeys(func(e *Entry) bool {
		return e.Type == "content"
	})
	assetKeys := plan.EntryKeys(func(e *Entry) bool {
		return e.Type == "asset"
	})

	if len(contentKeys) == 0 {
		t.Error("Expected content entries")
	}
	if len(assetKeys) == 0 {
		t.Error("Expected asset entries")
	}
}

func TestLayout_NestedContent(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html">
		<Asset name="images" src="*.png" pipe="copy" to="{FILENAME}.{EXT}"/>
	</Content>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithSource("blog/post.md", "# Post").
		WithSource("blog/image.png", "fake-png-data").
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "<html>{{content}}</html>").
		WithPipeline("md", BasicMarkdownPipeline()).
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

	if plan.LenEntries() < 2 {
		t.Errorf("Expected at least 2 entries (content + asset), got %d", plan.LenEntries())
	}
}

func TestLayout_Cache(t *testing.T) {
	cache := NewLayoutCache()

	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "<html>{{content}}</html>").
		WithPipeline("md", BasicMarkdownPipeline()).
		Build()
	defer cleanup()

	ctx := context.Background()
	key := LayoutKey{
		RelLayoutPath: "layout.xml",
		BaseDir:       ".",
	}

	layouts, err := proj.GetLayouts(".")
	if err != nil {
		t.Fatalf("Failed to get layouts: %v", err)
	}

	layout := layouts["layout.xml"]

	// First call should build
	tpl1, err := cache.GetOrBuild(ctx, proj.SchemeFS, key, layout)
	if err != nil {
		t.Fatalf("Failed to build template: %v", err)
	}

	// Second call should use cache
	tpl2, err := cache.GetOrBuild(ctx, proj.SchemeFS, key, layout)
	if err != nil {
		t.Fatalf("Failed to get cached template: %v", err)
	}

	if tpl1 != tpl2 {
		t.Error("Expected same template instance from cache")
	}
}

func TestLayout_CacheInvalidation(t *testing.T) {
	cache := NewLayoutCache()

	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "{{header}}{{content}}").
		WithScheme("header.html", "<header>v1</header>").
		WithPipeline("md", BasicMarkdownPipeline()).
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
	key := LayoutKey{
		RelLayoutPath: "layout.xml",
		BaseDir:       ".",
	}

	layouts, err := proj.GetLayouts(".")
	if err != nil {
		t.Fatalf("Failed to get layouts: %v", err)
	}

	layout := layouts["layout.xml"]

	// Build template
	_, err = cache.GetOrBuild(ctx, proj.SchemeFS, key, layout)
	if err != nil {
		t.Fatalf("Failed to build template: %v", err)
	}

	// Invalidate cache
	affected := cache.InvalidateAndGetAffected("header.html")

	if len(affected) == 0 {
		t.Error("Expected affected layouts after invalidation")
	}

	// Check that the key was invalidated
	found := false
	for _, k := range affected {
		if k.RelLayoutPath == key.RelLayoutPath && k.BaseDir == key.BaseDir {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected key to be in affected list")
	}
}

func TestLayout_GetDeps(t *testing.T) {
	cache := NewLayoutCache()

	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Fragment name="header" src="header.html" pipe="copy" />
	<Fragment name="footer" src="footer.html" pipe="copy" />
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "{{header}}{{content}}{{footer}}").
		WithScheme("header.html", "<header></header>").
		WithScheme("footer.html", "<footer></footer>").
		WithPipeline("md", BasicMarkdownPipeline()).
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
	key := LayoutKey{
		RelLayoutPath: "layout.xml",
		BaseDir:       ".",
	}

	layouts, err := proj.GetLayouts(".")
	if err != nil {
		t.Fatalf("Failed to get layouts: %v", err)
	}

	layout := layouts["layout.xml"]

	// Build template
	_, err = cache.GetOrBuild(ctx, proj.SchemeFS, key, layout)
	if err != nil {
		t.Fatalf("Failed to build template: %v", err)
	}

	// Get dependencies
	deps, ok := cache.GetDeps(key)
	if !ok {
		t.Fatal("Expected to find dependencies")
	}

	if len(deps) < 2 {
		t.Errorf("Expected at least 2 dependencies (fragments), got %d", len(deps))
	}
}

func TestLayout_HotReload(t *testing.T) {
	layoutXML := `<?xml version="1.0" encoding="UTF-8"?>
<Layout src="template.html">
	<Content name="content" src="*.md" pipe="md" to="{FILENAME}.html"/>
</Layout>`

	proj, cleanup := NewTestProject(t).
		WithScheme("layout.xml", layoutXML).
		WithScheme("template.html", "<html>{{content}}{{hotreload}}</html>").
		WithPipeline("md", BasicMarkdownPipeline()).
		WithConfig(func(c *CraeftConfig) {
			c.FsWatcher.HotReload = true
			c.FsWatcher.Enabled = true
		}).
		Build()
	defer cleanup()

	ctx := context.Background()
	layouts, err := proj.GetLayouts(".")
	if err != nil {
		t.Fatalf("Failed to get layouts: %v", err)
	}

	layout := layouts["layout.xml"]
	rendered, _, err := layout.Render(ctx, ".", proj.SchemeFS)
	if err != nil {
		t.Fatalf("Failed to render: %v", err)
	}

	// Should contain WebSocket client code
	if !strings.Contains(rendered, "WebSocket") {
		t.Error("Expected hot reload script in rendered output")
	}
}
