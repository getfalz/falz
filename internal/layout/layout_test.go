package layout

import (
	"encoding/xml"
	"reflect"
	"testing"

	log "github.com/sirupsen/logrus"
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
	<Fragment source="/path/to/fragment.html"/>
	<Content source="/path/to/content.md"/>
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
						&Fragment{XMLName: xml.Name{Local: "Fragment"}, Source: "/path/to/fragment.html"},
						&Content{XMLName: xml.Name{Local: "Content"}, Source: "/path/to/content.md"},
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
		log.Infof("Expected %+v, but got %+v", expected.Child, layout.Child)
		t.Errorf("Expected %+v, but got %+v", expected, layout)
	}
}

// Test function for parsing Layout with mixed Renderable children (Flow, Grid, Fragment, Content)
func TestParseMixedLayout(t *testing.T) {
	xmlData := `<Layout>
	<Flow direction="horizontal">
	<Item width="100" height="150">
	<Fragment source="/path/to/fragment.html"/>
	</Item>
	<Item width="100" height="150">
	<Grid columns="2" gap="5px">
	<Item width="50" height="50"/>
	<Item width="50" height="50"/>
	</Grid>
	</Item>
	<Item width="100" height="150">
	<Content source="/path/to/content.md"/>
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
						&Content{XMLName: xml.Name{Local: "Content"}, Source: "/path/to/content.md"},
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
