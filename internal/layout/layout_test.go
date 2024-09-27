package layout

import (
	"encoding/xml"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// Test function for parsing Layout with Flow and Grid as Renderable children
func TestParseLayoutWithFlowAndGrid(t *testing.T) {
	xmlData := `<Layout>
	<Flow direction="horizontal" align="center" wrap="true">
	<Item width="100" height="200" hexpand="true" vexpand="false" span="2">
	<Grid columns="3" gap="10px">
	<Item width="50" height="50"/>
	</Grid>
	</Item>
	</Flow>
	</Layout>`

	expected := Layout{
		XMLName: xml.Name{Local: "Layout"},
		Child: &Flow{
			XMLName:   xml.Name{Local: "Flow"},
			Direction: "horizontal",
			Align:     "center",
			Wrap:      true,
			Items: []Item{
				{
					XMLName: xml.Name{Local: "Item"},
					Width:   "100",
					Height:  "200",
					HExpand: true,
					VExpand: false,
					Span:    2,
					Children: []Renderable{
						&Grid{
							XMLName: xml.Name{Local: "Grid"},
							Columns: 3,
							Gap:     "10px",
							Items: []Item{
								{XMLName: xml.Name{Local: "Item"}, Width: "50", Height: "50"},
							},
						},
					},
				},
			},
		},
	}

	var layout Layout
	err := xml.Unmarshal([]byte(xmlData), &layout)
	if err != nil {
		t.Fatalf("Error unmarshaling XML: %v", err)
	}

	if !reflect.DeepEqual(layout, expected) {
		t.Errorf("Expected %+v, but got %+v", expected, layout)
	}
}

// Test function for parsing Layout with Fragment and Content as Renderable children
func TestParseLayoutWithFragmentAndContent(t *testing.T) {
	xmlData := `<Layout>
	<Flow direction="vertical" align="left">
	<Item width="200" height="300">
	<Fragment src="/path/to/fragment.html"/>
	<Content path="*.md"/>
	</Item>
	</Flow>
	</Layout>`

	expected := Layout{
		XMLName: xml.Name{Local: "Layout"},
		Child: &Flow{
			XMLName:   xml.Name{Local: "Flow"},
			Direction: "vertical",
			Align:     "left",
			Items: []Item{
				{
					XMLName: xml.Name{Local: "Item"},
					Width:   "200",
					Height:  "300",
					Children: []Renderable{
						&Fragment{XMLName: xml.Name{Local: "Fragment"}, Source: "/path/to/fragment.html", Pipeline: ""},
						&Content{XMLName: xml.Name{Local: "Content"}, Path: "*.md", Pipeline: ""},
					},
				},
			},
		},
	}

	var layout Layout
	err := xml.Unmarshal([]byte(xmlData), &layout)
	if err != nil {
		t.Fatalf("Error unmarshaling XML: %v", err)
	}

	if diff := cmp.Diff(layout, expected, cmp.AllowUnexported(Layout{})); diff != "" {
		t.Errorf("Mismatch (-expected +actual):\n%s", diff)
	}
}

// Test function for parsing Layout with mixed Renderable children (Flow, Grid, Fragment, Content)
func TestParseMixedLayout(t *testing.T) {
	xmlData := `<Layout>
	<Flow direction="horizontal">
	<Item width="100" height="150">
	<Fragment src="/path/to/fragment.html"/>
	</Item>
	<Item width="100" height="150">
	<Grid columns="2" gap="5px">
	<Item width="50" height="50"/>
	<Item width="50" height="50"/>
	</Grid>
	</Item>
	<Item width="100" height="150">
	<Content path="/path/to/content.md"/>
	</Item>
	</Flow>
	</Layout>`

	expected := Layout{
		XMLName: xml.Name{Local: "Layout"},
		Child: &Flow{
			XMLName:   xml.Name{Local: "Flow"},
			Direction: "horizontal",
			Items: []Item{
				{
					XMLName: xml.Name{Local: "Item"},
					Width:   "100",
					Height:  "150",
					Children: []Renderable{
						&Fragment{XMLName: xml.Name{Local: "Fragment"}, Source: "/path/to/fragment.html"},
					},
				},
				{
					XMLName: xml.Name{Local: "Item"},
					Width:   "100",
					Height:  "150",
					Children: []Renderable{
						&Grid{
							XMLName: xml.Name{Local: "Grid"},
							Columns: 2,
							Gap:     "5px",
							Items: []Item{
								{XMLName: xml.Name{Local: "Item"}, Width: "50", Height: "50"},
								{XMLName: xml.Name{Local: "Item"}, Width: "50", Height: "50"},
							},
						},
					},
				},
				{
					XMLName: xml.Name{Local: "Item"},
					Width:   "100",
					Height:  "150",
					Children: []Renderable{
						&Content{XMLName: xml.Name{Local: "Content"}, Path: "/path/to/content.md"},
					},
				},
			},
		},
	}

	var layout Layout
	err := xml.Unmarshal([]byte(xmlData), &layout)
	if err != nil {
		t.Fatalf("Error unmarshaling XML: %v", err)
	}

	if !reflect.DeepEqual(layout, expected) {
		t.Errorf("Expected %+v, but got %+v", expected, layout)
	}
}

func TestLayout_ContentPattern_SingleContent(t *testing.T) {
	layout := &Layout{
		Child: &Content{
			Path: "content/path/file.md",
		},
	}

	// Test when the layout contains only one content directly
	expected := "content/path/file.md"
	actual := layout.ContentPattern()

	if expected != actual {
		t.Errorf("Expected %s, but got %s", expected, actual)
	}
}

func TestLayout_ContentPattern_NestedFlow(t *testing.T) {
	flow := &Flow{
		Items: []Item{
			{
				Children: []Renderable{
					&Content{
						Path: "nested/content/path/file1.md",
					},
				},
			},
		},
	}

	layout := &Layout{
		Child: flow,
	}

	// Test when the content is nested within a flow
	expected := "nested/content/path/file1.md"
	actual := layout.ContentPattern()

	if expected != actual {
		t.Errorf("Expected %s, but got %s", expected, actual)
	}
}

func TestLayout_ContentPattern_NestedGrid(t *testing.T) {
	grid := &Grid{
		Items: []Item{
			{
				Children: []Renderable{
					&Content{
						Path: "nested/grid/path/file2.md",
					},
				},
			},
		},
	}

	layout := &Layout{
		Child: grid,
	}

	// Test when the content is nested within a grid
	expected := "nested/grid/path/file2.md"
	actual := layout.ContentPattern()

	if expected != actual {
		t.Errorf("Expected %s, but got %s", expected, actual)
	}
}

func TestLayout_ContentPattern_NestedFlowAndGrid(t *testing.T) {
	flow := &Flow{
		Items: []Item{
			{
				Children: []Renderable{
					&Grid{
						Items: []Item{
							{
								Children: []Renderable{
									&Content{
										Path: "deep/nested/path/file3.md",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	layout := &Layout{
		Child: flow,
	}

	// Test when the content is deeply nested in a flow and grid
	expected := "deep/nested/path/file3.md"
	actual := layout.ContentPattern()

	if expected != actual {
		t.Errorf("Expected %s, but got %s", expected, actual)
	}
}

func TestLayout_ContentPattern_NoContent(t *testing.T) {
	flow := &Flow{
		Items: []Item{
			{
				Children: []Renderable{},
			},
		},
	}

	layout := &Layout{
		Child: flow,
	}

	// Test when there is no content in the layout
	expected := ""
	actual := layout.ContentPattern()

	if expected != actual {
		t.Errorf("Expected %s, but got %s", expected, actual)
	}
}

func TestLayout_CollectContents_FromItem(t *testing.T) {
	item := Item{
		Children: []Renderable{
			&Content{
				Path: "item/content/path/file4.md",
			},
		},
	}

	// Test collecting contents directly from an item
	expected := "item/content/path/file4.md"
	actual := collectContents(item)

	if expected != actual {
		t.Errorf("Expected %s, but got %s", expected, actual)
	}
}

func TestLayout_CollectContents_ItemWithNestedFlow(t *testing.T) {
	item := Item{
		Children: []Renderable{
			&Flow{
				Items: []Item{
					{
						Children: []Renderable{
							&Content{
								Path: "item/flow/nested/content/file5.md",
							},
						},
					},
				},
			},
		},
	}

	// Test collecting contents from an item with a nested flow
	expected := "item/flow/nested/content/file5.md"
	actual := collectContents(item)

	if expected != actual {
		t.Errorf("Expected %s, but got %s", expected, actual)
	}
}
