package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/pipeline"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

const (
	pipeDir = "pipes"
)

const (
	baseTsconfig = `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "esModuleInterop": true,
    "noEmit": true,
    "skipLibCheck": true
}
	}`
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

	indexTS = `import { toHtml } from "md"

export function transform(this: tCtx, input: Buffer): string | Buffer {
    const text = input.toString('utf8')

    const { file, target, vars } = this
    console.log(` + "`" + `[${file.ext}] ${file.path} -> ${target}` + "`" + `)

    return toHtml(text)
}
`
)

var addCommand = &cli.Command{
	Name: "add",
	Commands: []*cli.Command{
		{
			Name:  "pipe",
			Usage: "Add a new pipe to the project",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "entrypoint",
					Usage: "Entry point TS file (must end in .ts)",
					Value: "index.ts",
				},
				// &cli.StringFlag{
				// 	Name:     "source",
				// 	Usage:    "Source file suffix (e.g. md, yaml) — no leading dot",
				// 	Required: false,
				// },
				// &cli.StringFlag{
				// 	Name:     "target",
				// 	Usage:    "Target file suffix (e.g. html, txt) — no leading dot",
				// 	Required: true,
				// },
			},
			Action: func(ctx context.Context, c *cli.Command) error {

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
				entry := c.String("entrypoint")
				if !strings.HasSuffix(entry, ".ts") {
					return fmt.Errorf("entry point must end with .ts")
				}

				config := &pipeline.PipelineConfig{
					PipelineName: name,
					EntryPoint:   entry,
				}

				out, err := yaml.Marshal(config)
				if err != nil {
					return fmt.Errorf("marshal pipe config: %w", err)
				}

				currPipeDir := filepath.Join(craeftDir, pipeDir, name)
				if err := os.MkdirAll(currPipeDir, 0755); err != nil {
					return fmt.Errorf("failed to create pipe directory: %w", err)
				}

				for _, f := range []struct {
					path    string
					content string
				}{
					{filepath.Join(craeftDir, pipeDir, "tsconfig.base.json"), baseTsconfig},
					{filepath.Join(currPipeDir, configFile), string(out)},
					{filepath.Join(currPipeDir, "tsconfig.json"), tsconfig},
					{filepath.Join(currPipeDir, "index.ts"), indexTS},
					{filepath.Join(currPipeDir, "package.json"), `{"name":"` + name + `","type":"module","dependencies":{}}`},
				} {
					if err := os.WriteFile(f.path, []byte(f.content), 0644); err != nil {
						return fmt.Errorf("write %s: %w", f.path, err)
					}
				}

				return nil
			},
		},
	},
}
