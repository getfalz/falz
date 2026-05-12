package main

import (
	"craeft/internal/log"
	"craeft/internal/project"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

func Graph(c *cli.Context) error {
	p, ok := c.App.Metadata["project"].(*project.Project)
	if !ok || p == nil {
		return errors.New("project not found in context")
	}

	defer p.Close()

	ctx, stop := setupSignalHandler()
	defer stop()

	g := p.Graph()
	source := project.NewSourceIndex(p.Source, p.Scheme, p.Layouts, project.IgnoreList(p.Config.Ignore))

	if err := source.Scan(ctx, g); err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	log.IInfo(log.Project).
		Int("nodes", len(g.AllNodes())).
		Msg("scan complete")

	out := os.Stdout
	if err := project.WriteGraphML(out, g); err != nil {
		return fmt.Errorf("write DOT: %w", err)
	}

	return nil
}
