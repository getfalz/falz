package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"craeft/internal/log"
	"craeft/internal/project"

	"github.com/urfave/cli/v3"
)

var buildCommand = &cli.Command{
	Name:  "build",
	Usage: "Build the project",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Save in-between files",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		config := loadConfig(c)
		config.Debug = c.Bool("debug")

		p, err := project.New(config)
		if err != nil {
			log.Fatal(fmt.Sprintf("Failed initializing the project: %v", err))
		}
		defer p.Close()

		printWelcome(c, config.SourcePath, config.OutputPath)

		ctx, stop := setupSignalHandler()
		defer stop()

		start := time.Now()
		err = p.Build(ctx)
		dur := time.Since(start)

		switch {
		case err == nil:
			log.Print("Build OK in %s\n", dur)
			return nil

		case errors.Is(err, context.Canceled):
			log.Print("Build canceled after %s\n", dur)
			return nil

		default:
			log.Error(fmt.Errorf("build failed after %s:\n%v", dur, err))
			return err
		}
	},
}
