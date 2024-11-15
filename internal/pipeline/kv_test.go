package pipeline

import "testing"

func TestKvPutAndGet(t *testing.T) {
	jsFunction := `
        async function testPutAndGet() {
            const kv = require("kv");
            const bucket = kv.use("testBucket");
            await bucket.put("key1", "value1");
            return await bucket.get("key1");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testPutAndGet", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	value, ok := result.Export().(string)
	if !ok {
		t.Fatalf("Expected string, got %T", result.Export())
	}

	expected := "value1"
	if value != expected {
		t.Errorf("Expected %v, got %v", expected, value)
	}
}

func TestKvDelete(t *testing.T) {
	jsFunction := `
        async function testDelete() {
            const kv = require("kv");
            const bucket = kv.use("testBucket");
            await bucket.put("key2", "value2");
            await bucket.delete("key2");

            try {
                return await bucket.get("key2");
            } catch (err) {
                return err;
            }
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testDelete", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	expectedError := "key not found"
	if result.String() != expectedError {
		t.Errorf("Expected error message '%v', got '%v'", expectedError, result.String())
	}
}

func TestKvNestedBuckets(t *testing.T) {
	jsFunction := `
        async function testNestedBuckets() {
            const kv = require("kv");
            const parentBucket = kv.use("parent");
            const childBucket = kv.use("parent.child");

            await childBucket.put("nestedKey", "nestedValue");

            return await kv.use("parent.child").get("nestedKey");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testNestedBuckets", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	value, ok := result.Export().(string)
	if !ok {
		t.Fatalf("Expected string, got %T", result.Export())
	}

	expected := "nestedValue"
	if value != expected {
		t.Errorf("Expected %v, got %v", expected, value)
	}
}
