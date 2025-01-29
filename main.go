package main

import (
	"fmt"
	"os"

	"craeft/cmd"
	"craeft/internal/project"
	"craeft/internal/utils"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func init() {
	log.SetFormatter(&utils.CraeftFormatter{
		TextFormatter: log.TextFormatter{
			ForceColors: true,
		},
	})
	log.SetOutput(os.Stdout)
}

var (
	Name        = "craeft"
	Description = "..."
	Version     = "0.1.0"
)

func main() {
	var verbosity int
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"V"},
		Usage:   "print only the version",
	}

	app := &cli.App{
		Name:                   Name,
		Usage:                  "A new ...",
		Version:                Version,
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
			setLogLevel(verbosity)

			currentPath, err := os.Getwd()
			if err != nil {
				log.Fatalf("Error getting current path: %v", err)
			}

			printWelcome(c)

			p, err := project.New(currentPath)
			if err != nil {
				log.Fatalf("Error loading config: %v", err)
			}

			p.Config.SourceDirectory = currentPath
			p.Config.OutputDirectory = c.String("outputPath")

			// i mean unicode. can you find something similar to a hammer, or a bench?

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
func printWelcome(cli *cli.Context) {
	fmt.Printf("⚒ %s %s\n", cli.App.Name, cli.App.Version)
	fmt.Printf("")
	if cli.Int("port") != 0 {
		fmt.Printf("Server running on port: %d\n", cli.Int("port"))
	}
	fmt.Printf("──────────────────────────────\n")
}

func setLogLevel(verbosity int) {
	switch verbosity {
	case -1:
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
