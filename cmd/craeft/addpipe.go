package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/pipeline"

	"github.com/manifoldco/promptui"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

//go:embed craeft.d.ts
var tsDoc string

//go:embed global.d.ts
var globalDoc string

const (
	pipeDir = "pipes"
)

const (
	tsconfig = `{
    "compilerOptions": {
        "target": "ES2020",
        "module": "commonjs",
        "strict": true,
        "esModuleInterop": true,
        "baseUrl": ".",
        "lib": [
            "ES2020"
        ]
    },
    "include": [
        "index.ts",
        "../../types/craeft.d.ts",
        "../../types/global.d.ts"
    ]
}`

	indexTS = `import { Parser } from "md"

async function transform(this: trfThis, input: string): Promise<string>{
    console.log(input)

    return input
}

async function index(this: idxThis, input: string) {
    return {}
}`
)

func AddPipe(c *cli.Context) error {
	if c.Args().Len() < 1 {
		return fmt.Errorf("please provide a name for the pipe")
	}

	if _, err := os.Stat(craeftDir); os.IsNotExist(err) {
		return fmt.Errorf("directory .craeft does not exist, please run 'craeft init' first")
	}

	if err := os.MkdirAll(filepath.Join(craeftDir, pipeDir), 0755); err != nil {
		return fmt.Errorf("failed to create .craeft/pipes directory: %w", err)
	}

	name := c.Args().First()
	config, err := buildPipeConfig(name)
	if err != nil {
		return fmt.Errorf("failed to build pipe config: %w", err)
	}
	out, err := yaml.Marshal(config)
	if err != nil {
		panic(err)
	}

	currPipeDir := filepath.Join(craeftDir, pipeDir, config.PipelineName)
	if err := os.MkdirAll(currPipeDir, 0755); err != nil {
		return fmt.Errorf("failed to create pipe directory: %w", err)
	}

	if err := os.WriteFile(filepath.Join(currPipeDir, configFile), out, 0644); err != nil {
		return fmt.Errorf("failed to write pipe config file: %w", err)
	}

	typesDir := filepath.Join(craeftDir, "types")
	if err := os.MkdirAll(typesDir, 0755); err != nil {
		return fmt.Errorf("failed to create types directory: %w", err)
	}

	dtsPath := filepath.Join(typesDir, "craeft.d.ts")

	if _, err := os.Stat(dtsPath); os.IsNotExist(err) {
		if err := os.WriteFile(dtsPath, []byte(tsDoc), 0644); err != nil {
			return fmt.Errorf("failed to write craeft.d.ts: %w", err)
		}

		if err := os.WriteFile(dtsPath, []byte(globalDoc), 0644); err != nil {
			return fmt.Errorf("failed to write global.d.ts: %w", err)
		}
	}

	if err := os.WriteFile(filepath.Join(currPipeDir, "tsconfig.json"), []byte(tsconfig), 0644); err != nil {
		return fmt.Errorf("failed to write tsconfig.json: %w", err)
	}

	if err := os.WriteFile(filepath.Join(currPipeDir, "index.ts"), []byte(indexTS), 0644); err != nil {
		return fmt.Errorf("failed to write index.ts: %w", err)
	}

	pkgJSON := `{
  "name": "` + config.PipelineName + `",
  "type": "module",
  "dependencies": {}
}`
	if err := os.WriteFile(filepath.Join(currPipeDir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}

	activatePrompt := promptui.Prompt{
		Label:     "Would you like to activate this pipe now?",
		IsConfirm: true,
	}
	output, err := activatePrompt.Run()
	if err != nil {
		return fmt.Errorf("prompt failed: %w", err)
	}

	fmt.Printf("output: %s", output)

	if strings.ToLower(output) == "y" || strings.ToLower(output) == "yes" {

	}

	return nil
}

func buildPipeConfig(name string) (*pipeline.PipelineConfig, error) {
	config := &pipeline.PipelineConfig{
		PipelineName: name,
	}

	entryPrompt := promptui.Prompt{
		Label:   "Entry point TS file (e.g. index.ts)",
		Default: "index.ts",
		Validate: func(input string) error {
			if input == "" {
				return fmt.Errorf("entry point cannot be empty")
			}
			if !strings.HasSuffix(input, ".ts") {
				return fmt.Errorf("entry point must end with .ts")
			}
			return nil
		},
	}
	entry, err := entryPrompt.Run()
	if err != nil {
		return nil, fmt.Errorf("entry point prompt failed: %w", err)
	}
	config.EntryPoint = entry

	sourcePrompt := promptui.Prompt{
		Label: "Source file suffix (e.g. md, yaml)",
		Validate: func(input string) error {
			if input == "" {
				return fmt.Errorf("source suffix cannot be empty")
			}
			if strings.HasPrefix(input, ".") {
				return fmt.Errorf("do not include a leading dot")
			}
			return nil
		},
	}
	source, err := sourcePrompt.Run()
	if err != nil {
		return nil, fmt.Errorf("source suffix prompt failed: %w", err)
	}
	config.SourceSuffix = source

	targetPrompt := promptui.Prompt{
		Label: "Target file suffix (e.g. html, txt)",
		Validate: func(input string) error {
			if input == "" {
				return fmt.Errorf("target suffix cannot be empty")
			}
			if strings.HasPrefix(input, ".") {
				return fmt.Errorf("do not include a leading dot")
			}
			return nil
		},
	}
	target, err := targetPrompt.Run()
	if err != nil {
		return nil, fmt.Errorf("target suffix prompt failed: %w", err)
	}
	config.TargetSuffix = target

	return config, nil
}
