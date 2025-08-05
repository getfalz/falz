package main

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/project"
	"fmt"
	"runtime"

	"github.com/urfave/cli/v2"
)

func Server(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok {
		log.Fatal("Project not found in context.")
	}

	ctx := context.Background()
	opt := project.BuildOptions{UseCache: false, Concurrency: runtime.NumCPU()}
	stats, err := p.Build(ctx, opt)
	if err != nil {
		log.Fatal(fmt.Sprintf("Can't build project: %v", err))
	}

	fmt.Printf("Build finished in %s\n", stats.AssetsBuilt)

	if p.Config.FsWatcher.Content {
		go p.Server.WatchFiles()
	}

	if p.Config.FsWatcher.Theme {
		go p.Server.WatchTheme()
	}

	if p.Config.FsWatcher.Pipes {
		go p.Server.WatchPipes()
	}

	if p.Config.FsWatcher.HotReload {
		p.Server.HotReload()
	}

	p.Server.ServeFiles()

	return nil
}
