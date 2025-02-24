package cmd

import (
	"craeft/internal/log"
	"craeft/internal/project"

	"github.com/urfave/cli/v2"
)

func Server(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok {
		log.Fatal("Project not found in context.")
	}

	p.Build()

	if p.Config.FsWatcher.Content {
		go p.Server.WatchFiles()
	}

	if p.Config.FsWatcher.Theme {
		go p.Server.WatchTheme()
	}

	if p.Config.FsWatcher.HotReload {
		p.Server.HotReload()
	}

	p.Server.ServeFiles()

	return nil
}
