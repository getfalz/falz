package cmd

import (
	"craeft/assets"
	"craeft/internal/project"
	"fmt"
	"net/http"
	"os"

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
		outputFolder := project.AppConfig.OutputDirectory

		// 1) compile files
		project.Build(project.AppConfig.SourceDirectory, project.AppConfig.OutputDirectory)

		// 2) start server
		fs := http.FileServer(http.Dir(outputFolder))

		http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets.EmbedFiles))))
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Log the requested path
			log.Infof("Requested: %s", r.URL.Path)

			if _, err := os.Stat(outputFolder + r.URL.Path); os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}

			w.Header().Add("Cache-Control", "max-age=604800")
			fs.ServeHTTP(w, r)
		})

		// http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// 	w.Write([]byte("Hello, welcome to the custom route!"))
		// })

		// Start the web server
		log.Infof("Server started at port %d\n", project.AppConfig.Server.Port)
		err := http.ListenAndServe(fmt.Sprintf(":%d", project.AppConfig.Server.Port), nil)
		if err != nil {
			log.Fatalf("Error starting server: %v", err)
		}

		// 3) start websocket?
		// 4) start file watcher
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().IntVar(&serverPort, "port", 0, "port for the server (default from config file)")
}
