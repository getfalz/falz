package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Namespace struct {
	DocLines []string
	Entries  []string
}

func extractJSAPI(pkgPath string) (map[string]*Namespace, []string, error) {
	cfg := &packages.Config{Mode: packages.NeedSyntax | packages.NeedTypes}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, nil, err
	}

	namespaces := make(map[string]*Namespace)
	var globalEntries []string
	var currentNamespace string

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, cg := range file.Comments {
				var docLines []string
				var funSignature string
				var foundJSAPI bool
				var foundNamespace string

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

						if strings.HasPrefix(line, "@namespace") {
							foundNamespace = strings.TrimSpace(strings.TrimPrefix(line, "@namespace"))
							continue
						}

						if strings.HasPrefix(line, "function") {
							funSignature = line
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
					apiEntry += " **/\n" + funSignature

					if foundNamespace != "" {
						// Define a new namespace
						currentNamespace = foundNamespace
						if _, exists := namespaces[currentNamespace]; !exists {
							namespaces[currentNamespace] = &Namespace{}
						}
						namespaces[currentNamespace].DocLines = append(namespaces[currentNamespace].DocLines, docLines...)
					} else if currentNamespace != "" {
						// Add the function into the current namespace
						namespaces[currentNamespace].Entries = append(namespaces[currentNamespace].Entries, apiEntry)
					} else {
						// No namespace, global
						globalEntries = append(globalEntries, apiEntry)
					}
				}
			}
		}
	}

	return namespaces, globalEntries, nil
}

func writeDTS(namespaces map[string]*Namespace, globalEntries []string, outputFile string) error {
	var sb strings.Builder
	sb.WriteString("declare global {\n")

	for _, entry := range globalEntries {
		sb.WriteString(entry)
		sb.WriteString("\n")
	}

	for nsName, ns := range namespaces {
		sb.WriteString(fmt.Sprintf("  namespace %s {\n", nsName))

		// Namespace-level doc (only if it has lines)
		if len(ns.DocLines) > 0 {
			sb.WriteString("    /**\n")
			for _, line := range ns.DocLines {
				sb.WriteString("     * " + line + "\n")
			}
			sb.WriteString("     **/\n")
		}

		for _, entry := range ns.Entries {
			lines := strings.Split(entry, "\n")

			insideDoc := false
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "/**" {
					sb.WriteString("    /**\n")
					insideDoc = true
					continue
				}
				if trimmed == "**/" {
					sb.WriteString("    **/\n")
					insideDoc = false
					continue
				}
				if insideDoc {
					sb.WriteString("     " + strings.TrimPrefix(line, "*") + "\n")
				} else {
					sb.WriteString("    " + line + "\n")
				}
			}
		}

		sb.WriteString("  }\n")
	}

	sb.WriteString("}\nexport {};\n")

	fmt.Println(sb.String())
	return os.WriteFile(outputFile, []byte(sb.String()), 0644)
}

func main() {
	namespaces, globalEntries, err := extractJSAPI("craeft/internal/pipeline/js")
	if err != nil {
		fmt.Println("Error extracting API:", err)
		return
	}

	if err := writeDTS(namespaces, globalEntries, "./gendoc/api.d.ts"); err != nil {
		fmt.Println("Error writing .d.ts:", err)
	} else {
		fmt.Println("Successfully generated goja.d.ts")
	}
}
