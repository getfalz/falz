package pipeline

import (
	"path/filepath"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestScript(t *testing.T) {
	config := &JSConfig{
		Root: filepath.Join("..", "..", "example"),
		Path: "test_script.js",
	}

	code := `
	function index(name) {
		console.log("console log: "+name);
		return name;
	}
    `

	env, err := NewJs(config, code)
	if err != nil {
		t.Fatal(err)
	}

	result, err := env.CallFunc("index", nil, env.vm.ToValue("Hello"))
	if err != nil {
		log.Errorf("CallFunc test return error: '%s'", err)
	}

	result2, err := env.CallFunc("index", nil, env.vm.ToValue("World"))
	if err != nil {
		log.Errorf("CallFunc test return error: '%s'", err)
	}

	t.Logf("Result: %s %s", result, result2)
}

func TestRequire(t *testing.T) {
	config := &JSConfig{
		Root: filepath.Join("..", "..", "example"),
		Path: "test_require.js",
	}

	code := `
	const fs = require("fs");
	function test_require() {
		return typeof fs === 'object' && "readFile" in fs
	}
    `

	env, err := NewJs(config, code)
	if err != nil {
		t.Fatal(err)
	}

	result, err := env.CallFunc("test_require", nil, nil)
	if err != nil {
		log.Errorf("CallFunc test_require return error: '%s'", err)
	}

	t.Logf("Result: %s", result)
}

func TestAsyncCall(t *testing.T) {
	config := &JSConfig{
		Root: filepath.Join("..", "..", "example"),
		Path: "test_async.js",
	}

	code := `
	const fs = require("fs");

	async function test_async() {
		const p1 = fs.readFile("test.md");
		const p2 = fs.readFile("other.md");

		const result1 = await p1;
		const result2 = await p2;

		return result1 + result2;
	}
    `

	env, err := NewJs(config, code)
	if err != nil {
		t.Fatal(err)
	}

	result, err := env.CallAsyncFunc("test_async", nil, nil)
	if err != nil {
		log.Errorf("CallFunc test_require return error: '%s'", err)
	}

	t.Logf("Result: %s", result)
}
