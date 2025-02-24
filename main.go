package main

import (
	"fmt"
	"os"

	"craeft/cmd"
	"craeft/internal/log"
	"craeft/internal/project"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

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
			&cli.BoolFlag{
				Name:  "silence",
				Usage: "Disable output",
				Value: false,
			},
		},
		Metadata: make(map[string]interface{}),
		Before: func(c *cli.Context) error {
			setLogLevel(verbosity, c.Bool("silence"))

			// TODO starting point is the current dir. craeft could search for the root folder
			currentPath, err := os.Getwd()
			if err != nil {
				log.Fatal(fmt.Sprintf("Error getting current path: %v", err))
			}

			printWelcome(c, currentPath)

			p, err := project.New(currentPath)
			if err != nil {
				log.Fatal(fmt.Sprintf("Error loading config: %v", err))
			}

			p.Config.SourceDirectory = currentPath
			p.Config.OutputDirectory = c.String("outputPath")

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
						Name:  "watch-content",
						Usage: "Enable hot-reload on the Content folder",
						Value: true,
					},
					&cli.BoolFlag{
						Name:  "watch-theme",
						Usage: "Enable hot-reload on the Theme folder",
						Value: false,
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
					p.Config.EnableHotReload = c.Bool("watch-content")
					p.Config.EnableThemeHotReload = c.Bool("watch-theme")

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
		log.Fatal(err.Error())
	}
}
func printWelcome(cli *cli.Context, base string) {
	fmt.Printf("⚒ %s %s\n", cli.App.Name, cli.App.Version)
	fmt.Printf("➤ Building folder %s\n", base)
	fmt.Printf("")
	if cli.Int("port") != 0 {
		fmt.Printf("Server running on port: %d\n", cli.Int("port"))
	}
	fmt.Printf("──────────────────────────────\n")
}

func setLogLevel(verbosity int, silence bool) {
	if silence {
		zerolog.SetGlobalLevel(zerolog.Disabled)
		return
	}

	switch verbosity {
	case -1:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case 1:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case 2:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case 3:
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	}
}
