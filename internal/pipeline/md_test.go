package pipeline

import (
	"testing"
)

func TestMdParse(t *testing.T) {
	jsFunction := `
        async function testAsyncParse() {
    		const md = require('md');
            const options = {
                parser: {
                    Tables: true,
                    FencedCode: true,
                    Autolink: true,
					AutoHeadingIDs: true
                },
                renderer: {
                    Smartypants: true
                }
            };

            const parser = new md.Parser(options);
            const markdownText = "# Async Markdown\n\nThis is a test.";
            return await parser.parse(markdownText);
        }
    `

	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.Call("testAsyncParse", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	value, ok := result.Export().(string)
	if !ok {
		t.Fatalf("Expected string, got %T", result.Export())
	}

	expected := `<h1 id="async-markdown">Async Markdown</h1>

<p>This is a test.</p>
`
	if value != expected {
		t.Errorf("Expected %v, got %v", expected, value)
	}
}
