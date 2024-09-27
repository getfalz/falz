package cmd

import (
	"context"
	"craeft/internal/project"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

var (
	outputPath string
	themePath  string
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
		currentPath, err := os.Getwd()
		if err != nil {
			log.Fatalf("Error getting current path: %v", err)
		}
		p, err := project.New(currentPath)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		p.Config.SourceDirectory = currentPath
		p.Config.OutputDirectory = outputPath

		if themePath != "" {
			p.Config.ThemePath = themePath
		} else {
			p.Config.ThemePath = currentPath
		}

		setLogLevel(verbose)

		ctx := context.WithValue(cmd.Context(), CTX, p)
		cmd.SetContext(ctx)
	},
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&outputPath, "output", "dist", "Build path of the project")
	rootCmd.PersistentFlags().StringVar(&themePath, "theme", "", "Theme path")
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "verbose loging")
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
