package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
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
	opt := project.BuildOptions{
		Concurrency:     runtime.GOMAXPROCS(0),
		WriteBufferSize: 50,
		UseCAS:          true,
	}

	stats, err := p.Build(ctx, opt)
	dur := time.Since(start)

	switch {
	case err == nil:
		log.Print("Build OK in %s — %s\n", dur, stats)
		return nil

	case errors.Is(err, context.Canceled):
		log.Print("Build canceled after %s — %s\n", dur, stats)
		return nil

	default:
		log.Error(fmt.Errorf("build failed after %s:\n%v", dur, err))
		return err
	}
}
