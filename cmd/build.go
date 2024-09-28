package cmd

import (
	"craeft/internal/project"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var debug bool

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	PreRun: func(cmd *cobra.Command, args []string) {
		cp := cmd.Context().Value(CTX)
		p, ok := cp.(*project.Project)
		if !ok {
			log.Error("Project not found in context.")
		}

		p.Config.Debug = debug
	},
	Run: func(cmd *cobra.Command, args []string) {
		cp := cmd.Context().Value(CTX)
		p, ok := cp.(*project.Project)
		if !ok {
			log.Error("Project not found in context.")
		}

		log.Infof("Building project from %s to %s\n", p.Config.SourceDirectory, p.Config.OutputDirectory)

		// TODO Build could be done parallel. First check for cache access
		p.Build(false)
		p.BuildAssets()
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().BoolVar(&debug, "debug", false, "Save in-between files")
}
