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
				Name:  "output",
				Value: "api.d.ts",
				Usage: "Build path of the documentation",
			},
		},
		Action: func(c *cli.Context) error {
			mod := c.Args().First()

			ns, err := tsdoc.Load(mod)
			if err != nil {
				os.Exit(1)
			}

			doc := tsdoc.WriteDTS(ns)
			fmt.Println("Generated TypeScript definitions:")
			fmt.Println(doc)

			if err := os.MkdirAll(filepath.Dir(c.String("output")), 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			if err := os.WriteFile(c.String("output"), []byte(doc), 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err.Error())
	}

}
