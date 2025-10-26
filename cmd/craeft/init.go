package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/project"

	"github.com/urfave/cli/v2"
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
	<Content name="body" src="*.txt"/>
</Layout>`
)

func Init(c *cli.Context) error {
	if _, err := os.Stat(craeftDir); !os.IsNotExist(err) {
		return fmt.Errorf("directory .craeft already exists")
	}

	if err := os.MkdirAll(filepath.Join(craeftDir, schemeDir), 0755); err != nil {
		return fmt.Errorf("failed to create .craeft directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(craeftDir, pipesDir), 0755); err != nil {
		return fmt.Errorf("failed to create .craeft directory: %w", err)
	}

	// config := &project.CraeftConfig{}

	// if err := defaults.Set(config); err != nil {
	// 	panic(err)
	// }

	config, err := buildProjectConfig(c)
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}
	out, err := yaml.Marshal(config)
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(filepath.Join(craeftDir, configFile), out, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := os.WriteFile(filepath.Join(craeftDir, schemeDir, layoutFile), []byte(layoutTpl), 0644); err != nil {
		return fmt.Errorf("failed to write layout file: %w", err)
	}

	return nil
}

func buildProjectConfig(c *cli.Context) (*project.CraeftConfig, error) {
	output := c.String("output")
	if output == "" {
		return nil, fmt.Errorf("output directory cannot be empty")
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory %q: %w", output, err)
	}

	raw := c.String("activate")
	var selected []string
	if raw != "" {
		opts := strings.Split(raw, ",")
		valid := map[string]bool{"md": true, "tpl": true}
		for _, r := range opts {
			name := strings.TrimSpace(r)
			if valid[name] {
				selected = append(selected, name)
			} else {
				// Warn but continue
				fmt.Fprintf(os.Stderr, "Warning: %q is not a known default pipe, skipping.\n", name)
			}
		}
	}

	return &project.CraeftConfig{
		OutputPath: output,
		Pipes:      selected,
	}, nil
}
