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

	log.Infof("Building project from %s to %s\n", p.Config.SourceDirectory, p.Config.OutputDirectory)

	// TODO Build could be done parallel. First check for cache access
	p.Build(false)
	p.BuildAssets()
	return nil
}
