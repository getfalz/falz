package cmd

import (
	"context"
	"craeft/internal/project"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

var (
	sourcePath string
	outputPath string
	verbose    int
)

const CTX = "craeft"

var rootCmd = &cobra.Command{
	Use:   "craeft",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		p, err := project.New(sourcePath)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		p.Config.SourceDirectory = sourcePath

		if outputPath != "" {
			p.Config.OutputDirectory = outputPath
		}

		setLogLevel(verbose)

		ctx := context.WithValue(cmd.Context(), CTX, p)
		cmd.SetContext(ctx)
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&sourcePath, "source", ".", "source path of the project")
	rootCmd.PersistentFlags().StringVar(&outputPath, "output", "", "output path of the project")
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "verbose output")
}

func setLogLevel(verbosity int) {
	switch verbosity {
	case 0:
		log.SetLevel(log.ErrorLevel)
	case 1:
		log.SetLevel(log.WarnLevel)
	case 2:
		log.SetLevel(log.InfoLevel)
	case 3:
		log.SetLevel(log.DebugLevel)
	default:
		log.SetLevel(log.TraceLevel)
	}
}
