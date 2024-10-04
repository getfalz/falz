package project

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"craeft/assets"

	"github.com/gorilla/websocket"
	"github.com/rjeczalik/notify"
	log "github.com/sirupsen/logrus"
)

type Server struct {
	config     *CraeftConfig
	project    *Project
	httpServer *http.Server
	routing    *http.ServeMux

	// websocket
	upgrader       websocket.Upgrader
	wsclients      map[*websocket.Conn]bool
	wsclientsMutex sync.Mutex
}

func NewServer(p *Project) *Server {
	return &Server{
		config:  p.Config,
		project: p,
		routing: http.NewServeMux(),

		wsclients: make(map[*websocket.Conn]bool),
	}
}

func (s *Server) ServeFiles() {
	fs := http.FileServer(http.Dir(s.config.OutputDirectory))

	s.routing.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets.EmbedFiles))))
	s.routing.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      s.routing,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Infof("Server started at port %d\n", s.config.Server.Port)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

			if s.project.ignoreFile(relPath) {
				continue
			}

			newPath, err := s.project.UpdatePath(relPath)
			if err != nil {
				log.Error("Can't resolve relative path", "error", err)
				continue
			}

			// TODO only change of source files. dist may be on the same folder
			newUrl, err := filepath.Rel(s.config.OutputDirectory, newPath)
			if err != nil {
				log.Error("Can't resolve absolute path", "error", err)
				continue
			}
			s.notifyPageChange(newUrl)

			// Handle the file change (e.g., rebuild, refresh, etc.)
		case notify.Remove, notify.Rename:
			log.Infof("TODO remove/rename builded file")
		}
	}
}

func (s *Server) HotReload() error {
	s.routing.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Errorf("Upgrade error:", err)
		}

		// keep connection open
		go func() {
			defer func() {
				s.wsclientsMutex.Lock()
				delete(s.wsclients, conn)
				s.wsclientsMutex.Unlock()
				conn.Close()
			}()

			for {
				mt, _, err := conn.ReadMessage()
				if err != nil || mt == websocket.CloseMessage {
					break
				}
			}
		}()

		s.wsclientsMutex.Lock()
		s.wsclients[conn] = true
		s.wsclientsMutex.Unlock()
	})

	return nil
}

func (s *Server) notifyPageChange(path string) error {
	message := map[string]string{"url": fmt.Sprintf("/%s", path)}

	s.wsclientsMutex.Lock()
	defer s.wsclientsMutex.Unlock()

	log.Infof("Notifying clients about page change: %s", path)
	for client := range s.wsclients {
		err := client.WriteJSON(message)
		if err != nil {
			client.Close()
			delete(s.wsclients, client)
		}
	}
	return nil
}

func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}
	log.Println("Server exited gracefully")
}
