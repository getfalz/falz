package cmd

import (
	"craeft/internal/project"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Infof("Building project from %s to %s\n", project.AppConfig.SourceDirectory, project.AppConfig.OutputDirectory)

		project.Build(project.AppConfig.SourceDirectory, project.AppConfig.OutputDirectory)
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
}
