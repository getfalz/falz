//go:generate go run ./gendoc/jsdocs.go
package main

import (
	_ "embed"
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
	Description = "A general purpose file to file converter"
	Version     = "0.1.0"
)

//go:embed gendoc/api.d.ts
var jsDocs string

func main() {
	var verbosity int
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"V"},
		Usage:   "print only the version",
	}

	app := &cli.App{
		Name:                   Name,
		Usage:                  Description,
		Version:                Version,
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "output",
				Usage: "Build path of the project",
			},
			&cli.StringFlag{
				Name:  "scheme",
				Usage: "Use the given scheme",
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
		Metadata: make(map[string]any),
		Before: func(c *cli.Context) error {
			setLogLevel(verbosity, c.Bool("silence"))

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
						Usage: "Enable file watcher on content folder",
						Value: true,
					},
					&cli.BoolFlag{
						Name:  "watch-theme",
						Usage: "Enable file watcher on theme folder",
						Value: false,
					},
					&cli.BoolFlag{
						Name:  "watch-pipes",
						Usage: "Enable file watcher on pipes folder",
						Value: false,
					},
					&cli.BoolFlag{
						Name:  "hot-reload",
						Usage: "Enable hot-reload. Only works if watch-content or watch-theme is enabled",
						Value: false,
					},
				},
				Before: func(c *cli.Context) error {
					initialize(c)
					p, ok := c.App.Metadata["project"].(*project.Project)
					if !ok {
						log.Fatal("Project not found in context.")
					}

					if c.Int("port") != 0 {
						p.Config.Server.Port = c.Int("port")
					}
					p.Config.FsWatcher.Content = c.Bool("watch-content")
					p.Config.FsWatcher.Theme = c.Bool("watch-theme")
					p.Config.FsWatcher.Pipes = c.Bool("watch-pipes")
					p.Config.FsWatcher.HotReload = (c.Bool("watch-content") || c.Bool("watch-theme") || c.Bool("watch-pipes")) && c.Bool("hot-reload")

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
					initialize(c)

					p, ok := c.App.Metadata["project"].(*project.Project)
					if !ok {
						log.Fatal("Project not found in context.")
					}

					p.Config.Debug = c.Bool("debug")

					return nil
				},
			},
			{
				Name:   "config",
				Usage:  "Generate a default config",
				Action: cmd.Config,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err.Error())
	}
}
func initialize(c *cli.Context) {
	// TODO starting point is the current dir. craeft could search for the root folder
	currentPath, err := os.Getwd()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error getting current path: %v", err))
	}

	printWelcome(c, currentPath, c.String("output"))

	config, err := project.LoadConfig(currentPath)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error loading config: %v", err))
	}

	config.SourcePath = currentPath
	if c.String("output") != "" {
		config.OutputPath = c.String("output")
	}
	if c.String("scheme") != "" {
		config.Scheme = c.String("scheme")
	}
	config.SetPaths()

	p, err := project.New(config)
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed initializing the project: %v", err))
	}

	c.App.Metadata["project"] = p
}

func printWelcome(cli *cli.Context, base, target string) {
	fmt.Printf("⚒ %s %s\n", cli.App.Name, cli.App.Version)
	fmt.Printf("➤ Building folder %q -> %q\n", base, target)
	fmt.Printf("")
	if cli.Int("port") != 0 {
		fmt.Printf("Server running on port: %d\n", cli.Int("port"))
		fmt.Printf("")
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
