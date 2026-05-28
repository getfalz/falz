//go:generate go run ../gendoc -e global -o craeft.d.ts craeft/internal/pipeline/js
//go:generate go run ../gendoc -i global -o global.d.ts craeft/internal/pipeline
//go:generate go run ../gendoc -o env.d.ts craeft/internal/content
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"craeft/internal/log"
	"craeft/internal/project"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
)

var (
	Name        = "craeft"
	Description = "A general purpose file to file converter"
	Version     = "0.1.0"
)

func main() {
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"V"},
		Usage:   "print only the version",
	}

	var verbosity int

	app := &cli.Command{
		Name:                   Name,
		Usage:                  Description,
		Version:                Version,
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "output",
				Usage: "Build path of the project",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "verbose logging",
				Config:  cli.BoolConfig{Count: &verbosity},
			},
			&cli.BoolFlag{
				Name:  "disable-cas",
				Usage: "Disable CAS caching",
				Value: false,
			},

			&cli.BoolFlag{
				Name:  "silence",
				Usage: "Disable output",
				Value: false,
			},
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			setLogLevel(int(c.Count("verbose")), c.Bool("silence"))
			return ctx, nil
		},
		Commands: []*cli.Command{
			serverCommand,
			buildCommand,
			graphCommand,
			configCommand,
			initCommand,
			addCommand,
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err.Error())
	}
}

func loadConfig(c *cli.Command) *project.CraeftConfig {
	currentPath, err := os.Getwd()
	if err != nil {
		log.Fatal(fmt.Sprintf("error getting current path: %v", err))
	}
	cfg, err := project.LoadConfig(currentPath)
	if err != nil {
		log.Fatal(fmt.Sprintf("error loading config: %v", err))
	}
	if c.String("output") != "" {
		cfg.OutputPath = c.String("output")
	}
	cfg.Build.UseCAS = !c.Bool("disable-cas")
	return cfg
}

func printWelcome(c *cli.Command, source, output string) {
	log.Print("⚒ %s %s\n", c.Root().Name, c.Root().Version)
	log.Print("➤ Building folder %q -> %q\n", source, output)
	log.Print("")
	log.Print("──────────────────────────────\n")
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

func setupSignalHandler() (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	go func() {
		<-ctx.Done()

		fmt.Fprintln(os.Stderr, "Shutting down gracefully… Press Ctrl+C again to force.")

		second := make(chan os.Signal, 1)
		signal.Notify(second, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(second)

		<-second
		fmt.Fprintln(os.Stderr, "Force exiting…")
		os.Exit(130)
	}()

	return ctx, stop
}
