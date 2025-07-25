package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

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
		},
		Action: func(c *cli.Context) error {
			mod := c.Args().First()

			ns, err := tsdoc.Load(mod, c.Bool("debug"))
			if err != nil {
				os.Exit(1)
			}

			doc := tsdoc.WriteDTS(ns)

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
