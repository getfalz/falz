package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/project"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

const (
	craeftDir  = ".craeft"
	configFile = "config.yaml"
	schemeDir  = "schemes"
	pipesDir   = "pipes"
	layoutFile = "layout.xml"
)

const (
	layoutTpl = `<Layout tpl="{{body}}">
	<Content name="body" src="*.txt" to="{FILENAME}.txt"/>
</Layout>`
)

var initCommand = &cli.Command{
	Name:  "init",
	Usage: "Generate a default config and initialize the project",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "output",
			Usage: "Output directory",
			Value: "dist",
		},
		&cli.StringFlag{
			Name:  "activate",
			Usage: `Which default pipes to activate? Comma-separated list of "md", "tpl"`,
			Value: "md",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		if _, err := os.Stat(craeftDir); !os.IsNotExist(err) {
			return fmt.Errorf("directory .craeft already exists")
		}

		if err := os.MkdirAll(filepath.Join(craeftDir, schemeDir), 0755); err != nil {
			return fmt.Errorf("failed to create schemes directory: %w", err)
		}
		if err := os.MkdirAll(filepath.Join(craeftDir, pipesDir), 0755); err != nil {
			return fmt.Errorf("failed to create pipes directory: %w", err)
		}

		output := c.String("output")
		if output == "" {
			return fmt.Errorf("output directory cannot be empty")
		}
		if err := os.MkdirAll(output, 0755); err != nil {
			return fmt.Errorf("failed to create output directory %q: %w", output, err)
		}

		pipes := parsePipes(c.String("activate"))
		config := &project.CraeftConfig{
			OutputPath: output,
			Pipes:      pipes,
		}

		out, err := yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}

		if err := os.WriteFile(filepath.Join(craeftDir, configFile), out, 0644); err != nil {
			return fmt.Errorf("write config: %w", err)
		}

		if err := os.WriteFile(filepath.Join(craeftDir, schemeDir, layoutFile), []byte(layoutTpl), 0644); err != nil {
			return fmt.Errorf("write layout: %w", err)
		}

		return nil
	},
}

func parsePipes(raw string) []string {
	if raw == "" {
		return nil
	}
	valid := map[string]bool{"md": true, "tpl": true}
	var selected []string
	for _, r := range strings.Split(raw, ",") {
		name := strings.TrimSpace(r)
		if valid[name] {
			selected = append(selected, name)
		} else {
			fmt.Fprintf(os.Stderr, "warning: %q is not a known default pipe, skipping\n", name)
		}
	}
	return selected
}
