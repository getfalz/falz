package main

import (
	"fmt"

	"craeft/internal/project"

	"github.com/creasty/defaults"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func Config(c *cli.Context) error {
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
}
