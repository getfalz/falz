package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"craeft/internal/log"
	"craeft/internal/project"
	"craeft/internal/server"

	"github.com/urfave/cli/v3"
)

var serverCommand = &cli.Command{
	Name:  "server",
	Usage: "Manage the server",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "port",
			Usage: "Port for the server",
		},
		&cli.BoolFlag{
			Name:  "watch",
			Usage: "Enable file watcher",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "hot-reload",
			Usage: "Enable hot-reload. Only works if watch-content or watch-theme is enabled",
			Value: false,
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		config := loadConfig(c)
		if port := c.Int("port"); port != 0 {
			config.Server.Port = int(port)
		}
		config.FsWatcher.Enabled = c.Bool("watch")
		config.FsWatcher.HotReload = c.Bool("watch") && c.Bool("hot-reload")

		p, err := project.New(config)
		if err != nil {
			log.Fatal(fmt.Sprintf("Failed initializing the project: %v", err))
		}
		defer p.Close()

		printWelcome(c, config.SourcePath, config.OutputPath)

		ctx, stop := setupSignalHandler()
		defer stop()

		start := time.Now()
		if err := server.RunServer(ctx, p); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error(fmt.Errorf("server failed after %s: %v", time.Since(start), err))
			return err
		}
		return nil

	},
}
