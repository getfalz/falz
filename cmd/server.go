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

	// 1) compile files
	p.Build()

	// 4) start file watcher
	go p.Server.WatchFiles()

	if p.Config.EnableThemeHotReload {
		go p.Server.WatchTheme()
	}

	// 2) start websocket?
	if p.Config.EnableHotReload {
		p.Server.HotReload()
	}
	// 3) start server
	p.Server.ServeFiles()

	return nil
}
