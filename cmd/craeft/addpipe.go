package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/pipeline"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

//go:embed craeft.d.ts
var tsDoc string

//go:embed global.d.ts
var globalDoc string

//go:embed env.d.ts
var envDoc string

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
        "../../types/*.d.ts",
    ]
}`

	indexTS = `import { Parser } from "md"

export async function transform(this: tCtx, input: string): Promise<string>{
    console.log(input)

    return input
}

export async function index(this: iCtx, input: string) {
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

	config, err := buildPipeConfig(c)
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

	if err := os.WriteFile(filepath.Join(typesDir, "craeft.d.ts"), []byte(tsDoc), 0644); err != nil {
		return fmt.Errorf("failed to write craeft.d.ts: %w", err)
	}

	if err := os.WriteFile(filepath.Join(typesDir, "global.d.ts"), []byte(globalDoc), 0644); err != nil {
		return fmt.Errorf("failed to write global.d.ts: %w", err)
	}
	if err := os.WriteFile(filepath.Join(typesDir, "env.d.ts"), []byte(envDoc), 0644); err != nil {
		return fmt.Errorf("failed to write env.d.ts: %w", err)
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

	return nil
}

func buildPipeConfig(c *cli.Context) (*pipeline.PipelineConfig, error) {
	name := c.Args().First()
	if name == "" {
		return nil, fmt.Errorf("pipeline name is required as the first argument")
	}
	entry := c.String("entrypoint")
	if entry == "" {
		return nil, fmt.Errorf("entry point cannot be empty")
	}
	if !strings.HasSuffix(entry, ".ts") {
		return nil, fmt.Errorf("entry point must end with .ts")
	}

	// source := c.String("source")
	// if source == "" {
	// 	return nil, fmt.Errorf("source suffix cannot be empty")
	// }
	// if strings.HasPrefix(source, ".") {
	// 	return nil, fmt.Errorf("source suffix must not start with a dot")
	// }
	//
	// target := c.String("target")
	// if target == "" {
	// 	return nil, fmt.Errorf("target suffix cannot be empty")
	// }
	// if strings.HasPrefix(target, ".") {
	// 	return nil, fmt.Errorf("target suffix must not start with a dot")
	// }

	return &pipeline.PipelineConfig{
		PipelineName: name,
		EntryPoint:   entry,
		// SourceSuffix: source,
		// TargetSuffix: target,
	}, nil
}
