package cmd

import (
	"craeft/internal/project"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

var serverPort int

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	PreRun: func(cmd *cobra.Command, args []string) {
		if serverPort != 0 {
			project.AppConfig.Server.Port = serverPort
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// TODO file watcher & serve
		fs := http.FileServer(http.Dir(project.AppConfig.OutputDirectory))
		http.Handle("/", addCacheHeader(fs))

		log.Infof("Server started at port %d\n", project.AppConfig.Server.Port)
		err := http.ListenAndServe(fmt.Sprintf(":%d", project.AppConfig.Server.Port), nil)
		if err == nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().IntVar(&serverPort, "port", 0, "port for the server (default from config file)")
}

func addCacheHeader(fs http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "max-age=604800")
		fs.ServeHTTP(w, r)
	}
}
