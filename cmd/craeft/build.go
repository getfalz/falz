package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"craeft/internal/log"
	"craeft/internal/project"

	"github.com/urfave/cli/v2"
)

func Build(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok {
		log.Error(errors.New("can't get project from context"))
	}

	// TODO Build could be done parallel. First check for cache access
	ctx := context.Background()
	opt := project.BuildOptions{UseCache: false, Concurrency: runtime.NumCPU()}
	stats, err := p.Build(ctx, opt)
	if err != nil {
		log.Fatal(fmt.Sprintf("Can't build project: %v", err))
	}
	fmt.Printf("Build finished in %s\n", stats.AssetsBuilt)
	return nil
}
