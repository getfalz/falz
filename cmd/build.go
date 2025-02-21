package cmd

import (
	"errors"
	"fmt"

	"craeft/internal/log"
	"craeft/internal/project"

	"github.com/urfave/cli/v2"
)

var debug bool

func Build(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok {
		log.Error(errors.New("Can't get project from context"))
	}

	// TODO Build could be done parallel. First check for cache access
	if err := p.Build(false); err != nil {
		log.Fatal(fmt.Sprintf("Can't build project: %v", err))
	}
	p.BuildAssets()
	return nil
}
