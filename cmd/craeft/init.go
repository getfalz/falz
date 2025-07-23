package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/project"

	"github.com/manifoldco/promptui"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

const (
	craeftDir  = ".craeft"
	configFile = "config.yaml"
	schemeDir  = "schemes"
	layoutFile = "layout.xml"
)

const (
	layoutTpl = `<Layout tpl="{{body}}">
	<Content name="body" path="*.txt"/>
</Layout>`
)

func Init(c *cli.Context) error {

	if _, err := os.Stat(craeftDir); !os.IsNotExist(err) {
		return fmt.Errorf("directory .craeft already exists")
	}

	if err := os.MkdirAll(filepath.Join(craeftDir, schemeDir), 0755); err != nil {
		return fmt.Errorf("failed to create .craeft directory: %w", err)
	}

	// config := &project.CraeftConfig{}

	// if err := defaults.Set(config); err != nil {
	// 	panic(err)
	// }

	config, err := buildConfig()
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

func buildConfig() (*project.CraeftConfig, error) {
	outputPrompt := promptui.Prompt{
		Label:   "Output directory",
		Default: "dist",
	}
	output, err := outputPrompt.Run()
	if err != nil {
		return nil, fmt.Errorf("prompt failed: %w", err)
	}

	if err := os.MkdirAll(output, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	prompt := promptui.Prompt{
		Label: "Which default pipes to activate? (comma-separated, e.g. \"md,tpl\")",
	}
	result, err := prompt.Run()
	if err != nil {
		log.Fatalf("Prompt failed: %v", err)
	}

	options := strings.Split(result, ",")
	validDefaults := map[string]bool{"md": true, "tpl": true}

	var selected []string
	for _, r := range options {
		name := strings.TrimSpace(r)
		if validDefaults[name] {
			selected = append(selected, name)
		} else {
			fmt.Printf("Warning: %q is not a known default pipe, skipping.\n", name)
		}
	}

	config := &project.CraeftConfig{
		OutputPath: output,
		Pipes:      selected,
	}

	return config, nil
}
