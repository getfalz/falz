package pipeline

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/dop251/goja"
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

	result, err := env.CallFunc("index", nil, env.ToValue("Hello"))
	if err != nil {
		log.Errorf("CallFunc test return error: '%s'", err)
	}

	result2, err := env.CallFunc("index", nil, env.ToValue("World"))
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
	async function test_async() {
  		console.log("Start of async test");

  		// Example 1: Basic async operation with Promise
  		const promiseTest = new Promise((resolve) => {
    		setTimeout(() => {
      		console.log("Promise resolved after 2 seconds");
      		resolve("Promise result");
    		}, 2000); // 2-second delay
  		});

  		console.log("Promise will be finished in 2sec. waiting..");
  		const result = await promiseTest;
  		console.log("Received from promise:", result);

  		// Example 2: Simulate concurrent async operations with Promise.all
  		const asyncTask1 = new Promise((resolve) => setTimeout(() => resolve("Task 1 complete"), 1000));
  		const asyncTask2 = new Promise((resolve) => setTimeout(() => resolve("Task 2 complete"), 1500));
  		const asyncTask3 = new Promise((resolve) => setTimeout(() => resolve("Task 3 complete"), 500));

    	console.log("Starting three concurrent tasks taking different times to complete...");
  		const results = await Promise.all([asyncTask1, asyncTask2, asyncTask3]);
  		console.log("All tasks completed:", results);

  		// Example 3: Asynchronous loop
  		console.log("Starting async loop...");
  		for (let i = 0; i < 3; i++) {
    		await new Promise((resolve) => setTimeout(() => {
      		console.log(` + "`Loop iteration ${i + 1} completed`" + `);
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

	_, err = env.CallAsyncFunc("test_async", nil, nil)
	if err != nil {
		log.Errorf("CallFunc test_require return error: '%s'", err)
	}

}

func TestAsyncGoFunc(t *testing.T) {
	config := &JSConfig{
		Root: filepath.Join("..", "..", "example"),
		Path: "test_async.js",
	}
	code_async := `
	async function test_async() {
  		const waitPromises = Array.from({ length: 5 }, () => wait());

  		const results = await Promise.all(waitPromises);
  		console.log("All waits completed:", results);
  		return results.join('');
	}
	`

	env, err := NewJs(config, code_async)
	if err != nil {
		t.Fatal(err)
	}

	env.loop.vm.Set("wait", func(call goja.FunctionCall) goja.Value {
		promise, resolve, _ := env.loop.vm.NewPromise()

		go func() {
			time.Sleep(time.Duration(1) * time.Second)
			resolve(env.loop.vm.ToValue("done"))
		}()

		return env.loop.vm.ToValue(promise)
	})

	startTime := time.Now()
	result, err := env.CallAsyncFunc("test_async", nil, nil)
	if err != nil {
		t.Fatalf("Failed to run code_async wait test: %v", err)
	}
	duration := time.Since(startTime)

	expectedResult := "donedonedonedonedone"
	fmt.Printf("result: %v\n", result)
	if result.String() != expectedResult {
		t.Fatalf("Expected %s, got %s", expectedResult, result.String())
	}

	if duration >= 1*time.Second && duration < 2*time.Second {
		t.Logf("Test passed, took approximately %v seconds as expected.", duration)
	} else {
		t.Fatalf("Test took %v, expected approximately 1 seconds", duration)
	}

}
func TestSyncGoFunc(t *testing.T) {
	config := &JSConfig{
		Root: filepath.Join("..", "..", "example"),
		Path: "test_sync.js",
	}

	code_sync := `
	async function test_sync() {
  		var result = '';
  		for (var i = 0; i < 5; i++) {
    		result += await wait();
  		}
  		return result;
	}
	`
	env, err := NewJs(config, code_sync)
	if err != nil {
		t.Fatal(err)
	}

	env.loop.vm.Set("wait", func(call goja.FunctionCall) goja.Value {
		promise, resolve, _ := env.loop.vm.NewPromise()

		go func() {
			time.Sleep(time.Duration(1) * time.Second)
			resolve(env.ToValue("done"))
		}()

		return env.ToValue(promise)
	})
	startTime := time.Now()
	result, err := env.CallAsyncFunc("test_sync", nil, nil)
	if err != nil {
		t.Fatalf("Failed to run code_sync wait test: %v", err)
	}
	duration := time.Since(startTime)

	expectedResult := "donedonedonedonedone"
	if result.String() != expectedResult {
		t.Fatalf("Expected %s, got %s", expectedResult, result.String())
	}

	if duration >= 5*time.Second && duration < 6*time.Second {
		t.Logf("Test passed, took approximately %v seconds as expected.", duration)
	} else {
		t.Fatalf("Test took %v, expected approximately 5 seconds", duration)
	}
}
