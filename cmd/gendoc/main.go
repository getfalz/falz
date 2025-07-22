package main

import (
	"fmt"
	"log"
	"os"

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
		Action: func(c *cli.Context) error {
			mod := c.Args().First()

			ns, err := tsdoc.Load(mod)
			if err != nil {
				os.Exit(1)
			}

			doc := tsdoc.WriteDTS(ns)
			fmt.Println("Generated TypeScript definitions:")
			fmt.Println(doc)

			return nil
			// entries, err := parseEntries("craeft/internal/pipeline/js")
			// if err != nil {
			// 	fmt.Fprintln(os.Stderr, "parse error:", err)
			// 	os.Exit(1)
			// }
			//
			// if err := writeDTS(entries, "./gendoc/api.d.ts"); err != nil {
			// 	fmt.Fprintln(os.Stderr, "write error:", err)
			// 	os.Exit(1)
			// }
			//
			// fmt.Println("Written ./gendoc/api.d.ts")

			// return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err.Error())
	}

}
