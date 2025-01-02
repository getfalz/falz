package main

import (
	"craeft/cmd"
	"craeft/internal/project"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func init() {
	log.SetOutput(os.Stdout)
}

func main() {
	var verbosity int
	app := &cli.App{
		Name:                   "craeft",
		Usage:                  "A new ...",
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "outputPath",
				Usage: "Build path of the project",
				Value: "dist",
			},
			&cli.StringFlag{
				Name:  "theme",
				Usage: "Theme name",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Usage:   "verbose logging",
				Aliases: []string{"v"},
				Count:   &verbosity,
			},
		},
		Metadata: make(map[string]interface{}),
		Before: func(c *cli.Context) error {
			currentPath, err := os.Getwd()
			if err != nil {
				log.Fatalf("Error getting current path: %v", err)
			}
			p, err := project.New(currentPath)
			if err != nil {
				log.Fatalf("Error loading config: %v", err)
			}

			p.Config.SourceDirectory = currentPath
			p.Config.OutputDirectory = c.String("outputPath")

			setLogLevel(verbosity)

			c.App.Metadata["project"] = p
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:   "server",
				Usage:  "Manage the server",
				Action: cmd.Server,
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "port",
						Usage: "Port for the server",
					},
					&cli.BoolFlag{
						Name:  "hotreload",
						Usage: "Enable hot-reload",
						Value: true,
					},
				},
				Before: func(c *cli.Context) error {
					p, ok := c.App.Metadata["project"].(*project.Project)
					if !ok {
						log.Fatal("Project not found in context.")
					}

					if c.Int("port") != 0 {
						p.Config.Server.Port = c.Int("port")
					}
					p.Config.EnableHotReload = c.Bool("hotreload")
					log.Infof("Using theme path: %s", p.Config.ThemePath)

					return nil
				},
			},
			{
				Name:   "build",
				Usage:  "Build commands",
				Action: cmd.Build,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Save in-between files",
					},
				},
				Before: func(c *cli.Context) error {
					p, ok := c.App.Metadata["project"].(*project.Project)
					if !ok {
						log.Fatal("Project not found in context.")
					}

					p.Config.Debug = c.Bool("debug")

					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func setLogLevel(verbosity int) {
	switch verbosity {
	case 0:
		log.SetLevel(log.WarnLevel)
	case 1:
		log.SetLevel(log.InfoLevel)
	case 2:
		log.SetLevel(log.DebugLevel)
	case 3:
		log.SetLevel(log.TraceLevel)
	default:
		log.SetLevel(log.TraceLevel)
	}
}
