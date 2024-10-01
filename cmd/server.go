package cmd

import (
	"craeft/internal/project"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func Server(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok {
		log.Fatal("Project not found in context.")
	}

	// 1) compile files
	p.Build(true)
	p.BuildAssets()

	// 4) start file watcher
	go p.Server.WatchFiles()

	// 2) start websocket?
	if p.Config.EnableHotReload {
		p.Server.HotReload()
	}
	// 3) start server
	p.Server.ServeFiles()
	return nil
}
