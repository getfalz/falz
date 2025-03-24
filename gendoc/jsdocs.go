package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

func extractJSAPI(pkgPath string) ([]string, error) {
	cfg := &packages.Config{Mode: packages.NeedSyntax | packages.NeedTypes}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, err
	}

	var apiEntries []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, cg := range file.Comments {
				var docLines []string
				var funName string
				var foundJSAPI bool

				for _, comment := range cg.List {
					line := comment.Text

					if strings.Contains(line, "@jsapi") {
						foundJSAPI = true
						continue
					}

					if foundJSAPI {
						if strings.HasPrefix(line, "//") {
							line = strings.TrimPrefix(line, "//")
							line = strings.TrimSpace(line)
						}

						if strings.HasPrefix(strings.TrimSpace(line), "function") {
							funName = line
							break
						}

						docLines = append(docLines, line)
					}
				}

				if foundJSAPI {
					apiEntry := "/**\n"
					for _, docLine := range docLines {
						apiEntry += " * " + strings.TrimSpace(docLine) + "\n"
					}
					apiEntry += " **/\n" + funName
					apiEntries = append(apiEntries, apiEntry)
				}
			}
		}
	}

	return apiEntries, nil
}

func writeDTS(apiEntries []string, outputFile string) error {
	var sb strings.Builder
	sb.WriteString("declare global {\n")
	sb.WriteString(strings.Join(apiEntries, "\n") + "\n")
	sb.WriteString("}\nexport {};")

	fmt.Println(sb.String())
	return os.WriteFile(outputFile, []byte(sb.String()), 0644)
}

func main() {
	apiEntries, err := extractJSAPI("craeft/internal/pipeline/js")
	if err != nil {
		fmt.Println("Error extracting API:", err)
		return
	}

	if err := writeDTS(apiEntries, "./gendoc/api.d.ts"); err != nil {
		fmt.Println("Error writing .d.ts:", err)
	} else {
		fmt.Println("Successfully generated goja.d.ts")
	}
}
