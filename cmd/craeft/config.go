package main

import (
	"context"
	"fmt"

	"craeft/internal/project"

	"github.com/creasty/defaults"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

var configCommand = &cli.Command{
	Name:  "config",
	Usage: "Generate a default config",
	Action: func(ctx context.Context, c *cli.Command) error {
		config := &project.CraeftConfig{}

		if err := defaults.Set(config); err != nil {
			panic(err)
		}

		out, err := yaml.Marshal(config)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(out))

		// TODO generate js docs to same folder

		return nil
	},
}
