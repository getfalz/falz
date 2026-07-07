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
	Usage: "Watch the project and rebuild on change",
	Flags: []cli.Flag{

		&cli.BoolFlag{
			Name:  "start-server",
			Usage: "Start the HTTP server",
		},
		&cli.IntFlag{
			Name:    "port",
			Aliases: []string{"p"},
			Usage:   "Port for the server",
		},
		&cli.BoolFlag{
			Name:  "hot-reload",
			Usage: "Enable live hot reload (requires the HTTP server)",
		},
		&cli.BoolFlag{
			Name:  "open",
			Usage: "Open the browser once the server is listening",
			Value: true,
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		config := loadConfig(c)
		if port := c.Int("port"); port != 0 {
			config.Server.Port = int(port)
		}
		config.Server.Enable = c.Bool("start-server") || config.Server.Enable
		config.Server.HotReload = c.Bool("hot-reload") || config.Server.HotReload
		if c.IsSet("port") {
			config.Server.Port = c.Int("port")
		}
		config.Server.OpenBrowser = c.Bool("open") && config.Server.Enable

		if c.Bool("clean") {
			if err := project.CleanCache(config); err != nil {
				log.Fatal(fmt.Sprintf("Failed cleaning cache: %v", err))
			}
			log.Print("Cleaned CAS and manifest\n")
		}

		p, err := project.New(config)
		if err != nil {
			log.Fatal(fmt.Sprintf("Failed initializing the project: %v", err))
		}
		defer p.Close()

		printWelcome(c, config.SourcePath, config.OutputPath)

		ctx, stop := setupSignalHandler()
		defer stop()

		start := time.Now()
		if err := server.RunServer(ctx, p, c.IsSet("port")); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error(fmt.Errorf("server failed after %s: %v", time.Since(start), err))
			return err
		}
		return nil

	},
}
