package cmd

import (
	"craeft/internal/project"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

var (
	sourcePath string
	outputPath string
	verbose    bool
)

var rootCmd = &cobra.Command{
	Use:   "craeft",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		configPath := filepath.Join(sourcePath, "config.yaml")

		log.Info("Loading config from ", configPath)
		err := project.LoadConfig(configPath)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		project.AppConfig.SourceDirectory = sourcePath

		if outputPath != "" {
			project.AppConfig.OutputDirectory = outputPath
		}

		if verbose {
			log.SetLevel(log.InfoLevel)
		} else {
			log.SetLevel(log.WarnLevel)
		}
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
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}
