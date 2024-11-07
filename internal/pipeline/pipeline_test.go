package pipeline

import (
	"testing"
)

func TestReadFile(t *testing.T) {
	// path := filepath.Join("..", "..", "example")
	// config := &JSConfig{
	// 	Root: path,
	// }
	//
	// loop := NewEventLoop()
	// vm := NewJs(config, loop)
	// vm.SetupRequire(config)
	//
	// script := `
	// 		const fs = require("fs");
	//
	// 		console.log(fs.readFile("test.md"))
	// 		fs.readFile("test.md")
	//    			.then(data => {
	//    				console.log("data"+data);
	//        			let text = new TextDecoder().decode(data);
	//    				console.log("text"+text);
	//    			})
	//    			.catch(err => {
	//        			console.error("Error:", err);
	//    			});
	//    `
	//
	// _, err := vm.RunString(script)
	// if err != nil {
	// 	t.Fatalf("Script execution failed: %v", err)
	// }
	// time.Sleep(2 * time.Second)
}
