package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"craeft/internal/log"
	"craeft/internal/project"
	"craeft/internal/server"

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
	if err := server.RunServer(ctx, p); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error(fmt.Errorf("server failed after %s: %v", time.Since(start), err))
		return err
	}
	return nil

}
