package main

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/project"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

var graphCommand = &cli.Command{
	Name:  "graph",
	Usage: "Scan the project and emit the dependency graph as Graphviz DOT",
	Action: func(ctx context.Context, c *cli.Command) error {
		config := loadConfig(c)

		p, err := project.New(config)
		if err != nil {
			log.Fatal(fmt.Sprintf("Failed initializing the project: %v", err))
		}
		defer p.Close()

		ctx, stop := setupSignalHandler()
		defer stop()

		g := p.Graph()
		source := project.NewSourceIndex(p.Source, p.Scheme, p.Layouts, p.Ignore)

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
	},
}
