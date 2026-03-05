package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"craeft/internal/log"
	"craeft/internal/project"

	"github.com/urfave/cli/v2"
)

func Build(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok || p == nil {
		log.Error(errors.New("can't get project from context"))
	}

	defer p.Close()

	ctx, stop := setupSignalHandler()
	defer stop()

	start := time.Now()

	err := p.Build(ctx)
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
}
