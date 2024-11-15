package pipeline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func getEnv(source string, t *testing.T) *JsEnv {
	config := &JSConfig{
		Root:   "testdata",
		Path:   "test_sync.js",
		DBPath: "testdata/test.db",
	}

	env, err := NewJs(config, source)
	if err != nil {
		t.Fatal(err)
	}

	return env
}

func TestFsBytes(t *testing.T) {
	testData := []byte("one line\n")

	jsFunction := `
        async function testBytes() {
            const fs = require("fs");
            return await fs.bytes("simple.md");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testBytes", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	bytesResult, ok := result.Export().([]byte)
	if !ok {
		t.Fatalf("Expected []byte, got %T", result.Export())
	}

	if !bytes.Equal(bytesResult, testData) {
		t.Errorf("Expected %v, got %v", testData, bytesResult)
	}
}

func TestFsWrite(t *testing.T) {
	jsFunction := `
        async function testWrite() {
            const fs = require("fs");
            const data = new Uint8Array([104, 101, 108, 108, 111]); // 'hello'
            await fs.write("testfile.txt", data);
        }
    `

	env := getEnv(jsFunction, t)
	defer env.Close()

	_, err := env.CallAsyncFunc("testWrite", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Join("testdata", "testfile.txt"))

	testFilePath := filepath.Join("testdata", "testfile.txt")
	data, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatal(err)
	}

	expectedData := []byte("hello")
	if !bytes.Equal(data, expectedData) {
		t.Errorf("Expected %v, got %v", expectedData, data)
	}
}

func TestFsAppend(t *testing.T) {
	testFilePath := filepath.Join("testdata", "testfile.txt")
	initialData := []byte("hello")
	err := os.WriteFile(testFilePath, initialData, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFilePath)

	jsFunction := `
        async function testAppend() {
            const fs = require("fs");
            const data = new Uint8Array([32, 119, 111, 114, 108, 100]); // ' world'
            await fs.append("testfile.txt", data);
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	_, err = env.CallAsyncFunc("testAppend", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatal(err)
	}

	expectedData := []byte("hello world")
	if !bytes.Equal(data, expectedData) {
		t.Errorf("Expected %v, got %v", expectedData, data)
	}
}

func TestFsIsFile(t *testing.T) {
	jsFunction := `
        async function testIsFile() {
            const fs = require("fs");
            return await fs.isFile("simple.md");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testIsFile", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	isFile, ok := result.Export().(bool)
	if !ok {
		t.Fatalf("Expected bool, got %T", result.Export())
	}

	if !isFile {
		t.Errorf("Expected true, got false")
	}
}

func TestFsIsDirectory(t *testing.T) {
	jsFunction := `
        async function testIsDirectory() {
            const fs = require("fs");
            return await fs.isDirectory("testdir");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testIsDirectory", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	isDir, ok := result.Export().(bool)
	if !ok {
		t.Fatalf("Expected bool, got %T", result.Export())
	}

	if !isDir {
		t.Errorf("Expected true, got false")
	}
}

func TestFsCreateDirectory(t *testing.T) {
	jsFunction := `
        async function testCreateDirectory() {
            const fs = require("fs");
            await fs.createDirectory("newdir");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	_, err := env.CallAsyncFunc("testCreateDirectory", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Join("testdata", "newdir"))

	testDirPath := filepath.Join("testdata", "newdir")
	info, err := os.Stat(testDirPath)
	if os.IsNotExist(err) {
		t.Fatalf("Expected directory to exist")
	}
	if !info.IsDir() {
		t.Errorf("Expected a directory, got a file")
	}
}

func TestFsDelete(t *testing.T) {
	testFilePath := filepath.Join("testdata", "testfile.txt")
	err := os.WriteFile(testFilePath, []byte("test"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	jsFunction := `
        async function testDelete() {
            const fs = require("fs");
            return await fs.delete("testfile.txt");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testDelete", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	deleted, ok := result.Export().(bool)
	if !ok {
		t.Fatalf("Expected bool, got %T", result.Export())
	}

	if !deleted {
		t.Errorf("Expected true, got false")
	}

	_, err = os.Stat(testFilePath)
	if !os.IsNotExist(err) {
		t.Errorf("Expected file to be deleted")
	}
}

func TestFsDeleteAll(t *testing.T) {
	testDirPath := filepath.Join("testdata", "testdel")
	err := os.Mkdir(testDirPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testDirPath)

	for i := 0; i < 3; i++ {
		filePath := filepath.Join(testDirPath, "file"+strconv.Itoa(i)+".txt")
		err := os.WriteFile(filePath, []byte("test"), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	jsFunction := `
        async function testDeleteAll() {
            const fs = require("fs");
            return await fs.deleteAll("testdel");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testDeleteAll", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	deleted, ok := result.Export().(bool)
	if !ok {
		t.Fatalf("Expected bool, got %T", result.Export())
	}

	if !deleted {
		t.Errorf("Expected true, got false")
	}

	_, err = os.Stat(testDirPath)
	if !os.IsNotExist(err) {
		t.Errorf("Expected directory to be deleted")
	}
}

func TestFsList(t *testing.T) {
	files := []string{"file1.txt", "file2.txt", "file3.txt"}
	jsFunction := `
        async function testList() {
            const fs = require("fs");
            const entries = [];
            // for await (const entry of fs.list("testdir")) {
            //     entries.push(entry.name);
            // }

    		const iterator = fs.list("testdir");
    		let result = await iterator.next();
    		while (!result.done) {
        		entries.push(result.value.name);
        		result = await iterator.next();
    		}
            return entries;
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testList", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	entryNames, ok := result.Export().([]interface{})
	if !ok {
		t.Fatalf("Expected []interface{}, got %T", result.Export())
	}

	if len(entryNames) != len(files) {
		t.Errorf("Expected %d entries, got %d", len(files), len(entryNames))
	}

	expectedFilesMap := make(map[string]bool)
	for _, fileName := range files {
		expectedFilesMap[fileName] = true
	}

	for _, entry := range entryNames {
		name, ok := entry.(string)
		if !ok {
			t.Errorf("Expected string, got %T", entry)
			continue
		}
		if !expectedFilesMap[name] {
			t.Errorf("Unexpected file: %s", name)
		}
	}
}

func TestFsSize(t *testing.T) {
	testData := []byte("one line\n")
	jsFunction := `
        async function testSize() {
            const fs = require("fs");
            return await fs.size("simple.md");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testSize", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	size, ok := result.Export().(int64)
	if !ok {
		t.Fatalf("Expected int64, got %T", result.Export())
	}

	if size != int64(len(testData)) {
		t.Errorf("Expected size %d, got %d", len(testData), size)
	}
}

func TestFsLastModified(t *testing.T) {
	testFilePath := filepath.Join("testdata", "testfile.txt")
	testData := []byte("hello world")
	err := os.WriteFile(testFilePath, testData, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFilePath)

	expectedTime := time.Now()

	err = os.Chtimes(testFilePath, expectedTime, expectedTime)
	if err != nil {
		t.Fatal(err)
	}

	jsFunction := `
        async function testLastModified() {
            const fs = require("fs");
            return await fs.lastModified("testfile.txt");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testLastModified", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	modifiedTime, ok := result.Export().(time.Time)
	if !ok {
		t.Fatalf("Expected time.Time, got %T", result.Export())
	}

	if !modifiedTime.Truncate(time.Second).Equal(expectedTime.Truncate(time.Second)) {
		t.Errorf("Expected %v, got %v", expectedTime, modifiedTime)
	}
}

func TestFsCopy(t *testing.T) {
	sourceData := []byte("one line\n")

	jsFunction := `
        async function testCopy() {
            const fs = require("fs");
            await fs.copy("simple.md", "dest.txt");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	_, err := env.CallAsyncFunc("testCopy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Join("testdata", "dest.txt"))

	destFilePath := filepath.Join("testdata", "dest.txt")
	destData, err := os.ReadFile(destFilePath)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(destData, sourceData) {
		t.Errorf("Expected %v, got %v", sourceData, destData)
	}
}

func TestFsCopyAll(t *testing.T) {
	files := []string{"file1.txt", "file2.txt", "file3.txt"}

	jsFunction := `
        async function testCopyAll() {
            const fs = require("fs");
            await fs.copyAll("testdir", "destdir");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	_, err := env.CallAsyncFunc("testCopyAll", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Join("testdata", "destdir"))

	destDirPath := filepath.Join("testdata", "destdir")
	for _, fileName := range files {
		destFilePath := filepath.Join(destDirPath, fileName)
		_, err := os.Stat(destFilePath)
		if os.IsNotExist(err) {
			t.Errorf("Expected file %s to exist in destination", fileName)
		}
	}
}

func TestFsMove(t *testing.T) {
	sourceFilePath := filepath.Join("testdata", "source.txt")
	sourceData := []byte("hello world")
	err := os.WriteFile(sourceFilePath, sourceData, 0644)
	if err != nil {
		t.Fatal(err)
	}

	jsFunction := `
        async function testMove() {
            const fs = require("fs");
            await fs.move("source.txt", "dest.txt");
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	_, err = env.CallAsyncFunc("testMove", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Join("testdata", "dest.txt"))

	_, err = os.Stat(sourceFilePath)
	if !os.IsNotExist(err) {
		t.Errorf("Expected source file to be moved")
	}

	destFilePath := filepath.Join("testdata", "dest.txt")
	destData, err := os.ReadFile(destFilePath)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(destData, sourceData) {
		t.Errorf("Expected %v, got %v", sourceData, destData)
	}
}

func TestFsMoveAll(t *testing.T) {
	sourceDirPath := filepath.Join("testdata", "sourcedir")
	err := os.Mkdir(sourceDirPath, 0755)
	if err != nil {
		t.Fatal(err)
	}

	files := []string{"file1.txt", "file2.txt"}
	for _, fileName := range files {
		filePath := filepath.Join(sourceDirPath, fileName)
		err := os.WriteFile(filePath, []byte("test"), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	jsFunction := `
        async function testMoveAll() {
            const fs = require("fs");
            await fs.moveAll("sourcedir", "destdir");
        }
    `

	env := getEnv(jsFunction, t)
	defer env.Close()

	_, err = env.CallAsyncFunc("testMoveAll", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Join("testdata", "destdir"))

	_, err = os.Stat(sourceDirPath)
	if !os.IsNotExist(err) {
		t.Errorf("Expected source directory to be moved")
	}

	destDirPath := filepath.Join("testdata", "destdir")
	for _, fileName := range files {
		destFilePath := filepath.Join(destDirPath, fileName)
		_, err := os.Stat(destFilePath)
		if os.IsNotExist(err) {
			t.Errorf("Expected file %s to exist in destination", fileName)
		}
	}
}

func TestFsText(t *testing.T) {
	testContent := "one line\n"
	jsFunction := `
        async function testText() {
            const fs = require("fs");
            const text = await fs.text("simple.md");
            return text;
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testText", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	textResult, ok := result.Export().(string)
	if !ok {
		t.Fatalf("Expected string, got %T", result.Export())
	}

	if textResult != testContent {
		t.Errorf("Expected '%s', got '%s'", testContent, textResult)
	}
}

func TestFsJSON(t *testing.T) {
	testData := map[string]interface{}{
		"key":    "value",
		"number": 42,
		"nested": map[string]interface{}{
			"bool": true,
		},
	}

	jsFunction := `
        async function testJSON() {
            const fs = require("fs");
            const jsonData = await fs.json("testfile.json");
            return jsonData;
        }
    `
	env := getEnv(jsFunction, t)
	defer env.Close()

	result, err := env.CallAsyncFunc("testJSON", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	jsonResult := result.Export()
	resultMap, ok := jsonResult.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map[string]interface{}, got %T", result.Export())
	}

	expectedJSON, _ := json.Marshal(testData)
	gotJSON, _ := json.Marshal(resultMap)
	if string(expectedJSON) != string(gotJSON) {
		t.Errorf("Expected %s, got %s", expectedJSON, gotJSON)
	}

}
