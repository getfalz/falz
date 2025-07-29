package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"craeft/pkg/tsdoc"

	"github.com/urfave/cli/v2"
)

var (
	Name        = "gendoc"
	Description = "Generate .d.ts files from go code comments"
	Version     = "0.1.0"
)

func main() {
	app := &cli.App{
		Name:                   Name,
		Usage:                  Description,
		Version:                Version,
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Build path of the documentation",
			},
			&cli.BoolFlag{
				Name:  "debug",
				Value: false,
				Usage: "Print debug information",
			},
			&cli.StringSliceFlag{
				Name:    "include",
				Aliases: []string{"i"},
				Usage:   "Comma-separated list of sections to include (md, tpl, kv)",
			},
			&cli.StringSliceFlag{
				Name:    "exclude",
				Aliases: []string{"e"},
				Usage:   "Comma-separated list of namespace names to exclude",
			},
		},
		Action: func(c *cli.Context) error {
			mod := c.Args().First()

			ns, err := tsdoc.Load(mod, c.Bool("debug"))
			if err != nil {
				os.Exit(1)
			}

			incs := c.StringSlice("include")
			excs := c.StringSlice("exclude")
			incMap := make(map[string]bool)

			if len(incs) == 0 {
				for _, n := range ns {
					incMap[n.Name] = true
				}
			} else {
				for _, v := range incs {
					name := strings.TrimSpace(v)
					if name != "" {
						incMap[name] = true
					}
				}
			}
			for _, v := range excs {
				name := strings.TrimSpace(v)
				if name != "" {
					delete(incMap, name)
				}
			}

			doc := tsdoc.WriteDTS(ns, incMap)

			if c.String("output") != "" {
				if err := os.MkdirAll(filepath.Dir(c.String("output")), 0755); err != nil {
					return fmt.Errorf("failed to create output directory: %w", err)
				}

				if err := os.WriteFile(c.String("output"), []byte(doc), 0644); err != nil {
					return fmt.Errorf("failed to write file: %w", err)
				}
			} else {
				fmt.Println(doc)
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err.Error())
	}

}
