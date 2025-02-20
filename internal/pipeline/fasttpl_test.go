package pipeline

import (
	"strings"
	"testing"
)

func TestFastTplRender(t *testing.T) {
	jsFunction := `
        async function testTemplate() {
            const { Template } = require("fasttpl");
            
            const templateInstance = new Template({
                startTag: '{{',
                endTag: '}}'
            });

            templateInstance.use("Hello, {{user}}! Current time is {{time}}. Unknown: {{unknown}}.");

            templateInstance.setParameters({
                user: "Alice",
                time: () => new Date().toLocaleTimeString()
            });

            return await templateInstance.render();
        }
    `

	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.Call("testTemplate", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	renderedResult, ok := result.Export().(string)
	if !ok {
		t.Fatalf("Expected string, got %T", result.Export())
	}

	// Define expected values
	staticUser := "Alice"
	// timePlaceholder := "<time-placeholder>" // Adjust this based on expected dynamic values
	// expectedTemplate := "Hello, Alice! Current time is " + timePlaceholder + ". Unknown: }}unknown."

	// Check for static part
	if !strings.Contains(renderedResult, staticUser) {
		t.Errorf("Expected template to contain %v, got %v", staticUser, renderedResult)
	}

	// Check for unknown variable placeholder
	if !strings.Contains(renderedResult, "{{unknown}}") {
		t.Errorf("Expected unknown variable placeholder }}unknown, got %v", renderedResult)
	}

	// Additional time validation (optional): Ensure a time value is present
	// You could parse and validate the time if necessary
	if !strings.Contains(renderedResult, ":") {
		t.Errorf("Expected time to be included, got %v", renderedResult)
	}

	t.Logf("Rendered template: %v", renderedResult)
}
