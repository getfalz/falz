package cmd

import (
	"craeft/internal/project"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	serverPort int
	dev        bool
)

var serverCmd = &cobra.Command{
	Use:   "server",
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
			log.Error("key not found in context")
		}

		if serverPort != 0 {
			p.Config.Server.Port = serverPort
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		cp := cmd.Context().Value(CTX)
		p, ok := cp.(*project.Project)
		if !ok {
			log.Error("key not found in context")
		}

		// 1) compile files
		p.Build(true)

		// 4) start file watcher
		go p.Server.WatchFiles()

		// 2) start websocket?

		// 3) start server
		p.Server.ServeFiles()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().IntVar(&serverPort, "port", 0, "port for the server (default from config file)")
	serverCmd.Flags().BoolVarP(&dev, "dev", "d", false, "Development output")
}
