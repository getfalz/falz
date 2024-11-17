package pipeline

import (
	"strings"
	"testing"
)

func TestHttpFetchGet(t *testing.T) {
	jsFunction := `
        async function testFetch() {
            const http = require("http");
            const response = await http.fetch("https://httpbin.org/get");
            const json = await response.json();
            return json.url;
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testFetch", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	urlResult, ok := result.Export().(string)
	if !ok {
		t.Fatalf("Expected string, got %T", result.Export())
	}

	expectedURL := "https://httpbin.org/get"
	if urlResult != expectedURL {
		t.Errorf("Expected %s, got %s", expectedURL, urlResult)
	}
}

func TestHttpHeadersMethods(t *testing.T) {
	jsFunction := `
        async function testHeaders() {
            const http = require("http");

            // Create a new Headers object
            const headers = new http.Headers();

            // Test set() method
            headers.set("Content-Type", "application/json");
            headers.set("Authorization", "Bearer token");

            // Test get() method
            const contentType = headers.get("Content-Type");
            const authorization = headers.get("Authorization");

            // Test has() method
            const hasContentType = headers.has("Content-Type");
            const hasMissingHeader = headers.has("Missing-Header");

            // Test delete() method
            headers.delete("Authorization");
            const deletedAuthorization = headers.get("Authorization");

            return {
                contentType,
                authorization,
                hasContentType,
                hasMissingHeader,
                deletedAuthorization
            };
        }
    `

	env := getEnv(jsFunction, t)
	defer env.Close()

	// Execute the JavaScript function
	result, err := env.CallAsyncFunc("testHeaders", nil, nil)
	if err != nil {
		t.Fatal("Error calling testHeaders:", err)
	}

	// Verify the result is an object
	obj := result.ToObject(env.loop.vm)
	if obj == nil {
		t.Fatal("Expected an object, got nil")
	}

	// Extract results from the returned object
	contentType := obj.Get("contentType").String()
	authorization := obj.Get("authorization").String()
	hasContentType := obj.Get("hasContentType").ToBoolean()
	hasMissingHeader := obj.Get("hasMissingHeader").ToBoolean()
	deletedAuthorization := obj.Get("deletedAuthorization").Export()

	// Check the results
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type to be 'application/json', got '%s'", contentType)
	}

	if authorization != "Bearer token" {
		t.Errorf("Expected Authorization to be 'Bearer token', got '%s'", authorization)
	}

	if !hasContentType {
		t.Errorf("Expected hasContentType to be true, got false")
	}

	if hasMissingHeader {
		t.Errorf("Expected hasMissingHeader to be false, got true")
	}

	if deletedAuthorization != nil {
		t.Errorf("Expected deletedAuthorization to be null, got '%v'", deletedAuthorization)
	}
}
func TestHttpRequestObject(t *testing.T) {
	jsFunction := `
        async function testRequest() {
            const http = require("http");
            const request = new http.Request("https://httpbin.org/post", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ key: "value" })
            });

            o = {
                url: request.url,
                method: request.method,
                headers: request.headers.get("Content-Type"),
                bodyUsed: request.bodyUsed
            };

			return o;
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testRequest", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	obj := result.ToObject(env.loop.vm)
	if obj == nil {
		t.Fatal("Expected object, got nil")
	}

	url := obj.Get("url").String()
	method := obj.Get("method").String()
	contentType := obj.Get("headers").String()
	bodyUsed := obj.Get("bodyUsed").Export().(bool)

	if url != "https://httpbin.org/post" {
		t.Errorf("Expected URL https://httpbin.org/post, got %s", url)
	}
	if method != "POST" {
		t.Errorf("Expected method POST, got %s", method)
	}
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
	if bodyUsed {
		t.Errorf("Expected bodyUsed to be false, got %v", bodyUsed)
	}
}

func TestHttpResponseObject(t *testing.T) {
	jsFunction := `
        async function testResponse() {
            const http = require("http");
            const response = await http.fetch("https://httpbin.org/get");
            const status = response.status;
            const statusText = response.statusText;
            const contentType = response.headers.get("Content-Type");
            const json = await response.json();

            return {
                status,
                statusText,
                contentType,
                json
            };
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testResponse", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	obj := result.ToObject(env.loop.vm)
	if obj == nil {
		t.Fatal("Expected object, got nil")
	}

	status := obj.Get("status").ToInteger()
	statusText := obj.Get("statusText").String()
	contentType := obj.Get("contentType").String()
	jsonObj := obj.Get("json").ToObject(env.loop.vm)

	if status != 200 {
		t.Errorf("Expected status 200, got %d", status)
	}
	if statusText != "200 OK" {
		t.Errorf("Expected statusText OK, got %s", statusText)
	}
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type to contain application/json, got %s", contentType)
	}
	if jsonObj.Get("url").String() != "https://httpbin.org/get" {
		t.Errorf("Expected JSON response to contain URL, got %s", jsonObj.Get("url").String())
	}
}

func TestHttpFetchPost(t *testing.T) {
	jsFunction := `
        async function testFetchPost() {
            const http = require("http");

            const headers = new http.Headers();
            headers.set("Content-Type", "application/json");

            const request = new http.Request("https://httpbin.org/post", {
                method: "POST",
                headers: headers,
                body: JSON.stringify({ name: "John Doe", age: 30 })
            });

            const response = await http.fetch(request);
            const json = await response.json();

            return json.json;
        }
    `

	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testFetchPost", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	responseData, ok := result.Export().(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map, got %T", result.Export())
	}

	name, nameOk := responseData["name"].(string)
	age, ageOk := responseData["age"].(float64)
	if !nameOk || !ageOk {
		t.Fatal("Response data missing expected fields")
	}

	if name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got %s", name)
	}

	if age != 30 {
		t.Errorf("Expected age 30, got %f", age)
	}
}
