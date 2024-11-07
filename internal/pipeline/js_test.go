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
  		console.log("Start of async test");

  		// Example 1: Basic async operation with Promise
  		const promiseTest = new Promise((resolve) => {
    		setTimeout(() => {
      		console.log("Promise resolved after 2 seconds");
      		resolve("Promise result");
    		}, 2000); // 2-second delay
  		});

  		console.log("Waiting for promise...");
  		const result = await promiseTest;
  		console.log("Received from promise:", result);

  		// Example 2: Simulate concurrent async operations with Promise.all
  		const asyncTask1 = new Promise((resolve) => setTimeout(() => resolve("Task 1 complete"), 1000));
  		const asyncTask2 = new Promise((resolve) => setTimeout(() => resolve("Task 2 complete"), 1500));
  		const asyncTask3 = new Promise((resolve) => setTimeout(() => resolve("Task 3 complete"), 500));

  		console.log("Starting concurrent tasks...");
  		const results = await Promise.all([asyncTask1, asyncTask2, asyncTask3]);
  		console.log("All tasks completed:", results);

  		// Example 3: Asynchronous loop
  		console.log("Starting async loop...");
  		for (let i = 0; i < 3; i++) {
    		await new Promise((resolve) => setTimeout(() => {
      		console.log('Loop iteration ${i + 1} completed');
      		resolve();
    		}, 1000));
  		}

  		console.log("End of async test");
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
