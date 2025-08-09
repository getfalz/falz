package main

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/project"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"
)

func Server(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok {
		return errors.New("project not found in context")
	}
	defer p.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	second := make(chan os.Signal, 1)
	signal.Notify(second, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		<-second
		fmt.Fprintln(os.Stderr, "Force exiting…")
		os.Exit(130)
	}()

	start := time.Now()
	if err := project.RunServer(ctx, p); err != nil {
		log.Error(fmt.Errorf("server failed after %s: %v", time.Since(start), err))
		return err
	}
	return nil

}
