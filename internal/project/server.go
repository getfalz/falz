package project

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"craeft/assets"

	"github.com/rjeczalik/notify"
	log "github.com/sirupsen/logrus"
)

type Server struct {
	config  *CraeftConfig
	project *Project
}

func (s *Server) ServeFiles() {
	fs := http.FileServer(http.Dir(s.config.OutputDirectory))

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets.EmbedFiles))))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Log the requested path
		log.Infof("Requested: %s", r.URL.Path)

		if _, err := os.Stat(s.config.OutputDirectory + r.URL.Path); os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}

		// w.Header().Add("Cache-Control", "max-age=604800")
		fs.ServeHTTP(w, r)
	})

	// http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
	// 	w.Write([]byte("Hello, welcome to the custom route!"))
	// })

	// Start the web server
	log.Infof("Server started at port %d\n", s.config.Server.Port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", s.config.Server.Port), nil)
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

func (s *Server) WatchFiles() {
	events := make(chan notify.EventInfo, 1)

	absPath, err := filepath.Abs(s.config.SourceDirectory)
	if err != nil {
		log.Fatal(err)
	}

	if err := notify.Watch(absPath+"/...", events, notify.All); err != nil {
		log.Fatalf("Can't watch files for change: %v", err)
	}
	defer notify.Stop(events)

	for {
		switch ei := <-events; ei.Event() {
		case notify.Write:
			relPath, err := filepath.Rel(absPath, ei.Path())
			if err != nil {
				log.Error("Can't resolve relative path", "error", err)
				continue
			}
			log.Infof("File changed. UPDATE: %s", relPath)

			err = s.project.BuildPage(relPath)
			if err != nil {
				log.Error("Can't resolve relative path", "error", err)
				continue
			}

			// Handle the file change (e.g., rebuild, refresh, etc.)
		case notify.Remove, notify.Rename:
			log.Infof("TODO remove/rename builded file")
		}
	}
}
