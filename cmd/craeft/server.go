package main

import (
	"craeft/internal/log"
	"craeft/internal/project"
	"errors"
	"fmt"
	"time"

	"github.com/urfave/cli/v2"
)

func Server(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok {
		return errors.New("project not found in context")
	}
	defer p.Close()

	ctx, stop := setupSignalHandler()
	defer stop()

	start := time.Now()
	if err := project.RunServer(ctx, p); err != nil {
		log.Error(fmt.Errorf("server failed after %s: %v", time.Since(start), err))
		return err
	}
	return nil

}
