package pipeline

import (
	"strings"
	"testing"
)

func normalize(input string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(input, " ", ""), "\t", ""), "\n", "")
}

func TestTplRender(t *testing.T) {
	jsFunction := `
async function testTemplate() {
    const { render } = require("tpl");

	html = ` + "`" + `<html>
	<%= if (names && len(names) > 0) { %>
		<ul>
			<%= for (n) in names { %>
				<li><%= capitalize(n) %></li>
			<% } %>
		</ul>
	<% } else { %>
		<h1>Sorry, no names. :(</h1>
	<% } %>
	</html>` + "`" + `

	vars = {
		names: ["john", "paul", "george", "ringo"],
	}

    return await render(html, vars);
}
    `

	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testTemplate", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	renderedResult, ok := result.Export().(string)
	if !ok {
		t.Fatalf("Expected string, got %T", result.Export())
	}

	expected := `<html>
		<ul>
			<li>John</li>
			<li>Paul</li>
			<li>George</li>
			<li>Ringo</li>
		</ul>
	</html>`

	if normalize(expected) != normalize(renderedResult) {
		t.Errorf("Expected template to contain\n %v\n, got\n %v\n", expected, renderedResult)
	}

	t.Logf("Rendered template: %v", renderedResult)
}
