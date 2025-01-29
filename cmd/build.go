package cmd

import (
	"craeft/internal/project"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var debug bool

func Build(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok {
		log.Fatal("Project not found in context.")
	}

	// TODO Build could be done parallel. First check for cache access
	if err := p.Build(false); err != nil {
		log.Fatal("Can't build project: ", err)
	}
	p.BuildAssets()
	return nil
}
