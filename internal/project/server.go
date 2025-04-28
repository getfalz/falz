package project

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"craeft/internal/log"

	"github.com/gorilla/websocket"
	"github.com/rjeczalik/notify"
)

type Server struct {
	config     *CraeftConfig
	project    *Project
	httpServer *http.Server
	routing    *http.ServeMux

	ignoreWrite int32

	// websocket
	upgrader       websocket.Upgrader
	wsclients      map[*websocket.Conn]bool
	wsclientsMutex sync.Mutex
}

func NewServer(p *Project) *Server {
	s := &Server{
		config:      p.Config,
		project:     p,
		routing:     http.NewServeMux(),
		ignoreWrite: 0,

		wsclients: make(map[*websocket.Conn]bool),
	}

	return s
}

func (s *Server) ServeFiles() {
	fs := http.FileServer(http.Dir(s.config.OutputPath))

	s.routing.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Info(fmt.Sprintf("Requested: %s", r.URL.Path))

		if _, err := os.Stat(s.config.OutputPath + r.URL.Path); os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}

		// w.Header().Add("Cache-Control", "max-age=604800")
		fs.ServeHTTP(w, r)
	})

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      s.routing,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Info(fmt.Sprintf("Server started at port %d", s.config.Server.Port))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(fmt.Sprintf("Error starting server: %v", err))
	}
}

type HotReloadMsg struct {
	Reload bool   `json:"reload"`
	Url    string `json:"url"`
}

// WatchFiles watches for changes in the source directory and triggers a rebuild
// what it don't watch is changes on the config file. it also only watch for file changes
// be careful, some files may end into multiple files, in this case, the websocket is notified with the last build file
func (s *Server) WatchFiles() {
	events := make(chan notify.EventInfo, 1)

	absPath, err := filepath.Abs(s.config.SourcePath)
	if err != nil {
		log.Fatal(err.Error())
	}

	if err := notify.Watch(absPath+"/...", events, notify.Write); err != nil {
		log.Fatal(fmt.Sprintf("Can't watch files for change: %v", err))
	}
	defer notify.Stop(events)

	for {
		ei := <-events

		if atomic.LoadInt32(&s.ignoreWrite) == 1 {
			continue
		}

		if filepath.Base(ei.Path()) == "config.yaml" {
			continue

		}
		switch ei.Event() {
		case notify.Write:
			relPath, err := filepath.Rel(absPath, ei.Path())
			if err != nil {
				log.Warn(fmt.Sprintf("Can't resolve relative path: %s", err))
				continue
			}

			if s.project.ignoreFile(relPath) {
				continue
			}

			log.Trace(fmt.Sprintf("File changed. UPDATE: %s", relPath))

			newPath, err := s.project.BuildPath(relPath)
			if err != nil {
				log.Error(err)
				continue
			}

			newUrl, err := filepath.Rel(s.config.OutputPath, newPath)
			if err != nil {
				log.Error(err)
				continue
			}

			if s.config.FsWatcher.HotReload {
				message := HotReloadMsg{Url: fmt.Sprintf("/%s", newUrl), Reload: false}
				s.sendToClient(message)
			}

		}
	}
}

func (s *Server) WatchTheme() {
	events := make(chan notify.EventInfo, 1)

	configPath, err := filepath.Abs(s.config.SchemePath)
	fmt.Println("server theme on path %s", s.config.SchemePath)
	if err != nil {
		log.Fatal(err.Error())
	}

	if err := notify.Watch(configPath+"/...", events, notify.Write); err != nil {
		log.Fatal(fmt.Sprintf("Can't watch files for change: %v", err))
	}
	defer notify.Stop(events)

	for {
		ei := <-events

		// TODO for now, we just rebuild all files. We could do better, and just rebuild the affected files
		log.Trace(fmt.Sprintf("Theme changed. Reloading file: %s", ei.Path()))
		atomic.StoreInt32(&s.ignoreWrite, 1)
		s.project.Build(false)
		atomic.StoreInt32(&s.ignoreWrite, 0)

		if s.config.FsWatcher.HotReload {
			s.reloadPage()
		}
	}
}

func (s *Server) WatchPipes() {
	events := make(chan notify.EventInfo, 1)

	configPath, err := filepath.Abs(s.config.PipesPath)
	fmt.Println("server theme on path %s", s.config.PipesPath)
	if err != nil {
		log.Fatal(err.Error())
	}

	// TODO: start a watcher for each pipe folder. this way, we can:
	// * selectively watch for pipes in ~/.config and .craeft/pipes
	// * when a pipe change, we can use the pipe information to just rebuild files transformed by the pipe
	if err := notify.Watch(configPath+"/...", events, notify.Write); err != nil {
		log.Fatal(fmt.Sprintf("Can't watch files for change: %v", err))
	}
	defer notify.Stop(events)

	for {
		ei := <-events

		// TODO for now, we just rebuild all files. We could do better, and just rebuild the affected files
		for _ = range s.config.Pipes {
			atomic.StoreInt32(&s.ignoreWrite, 1)
			log.Trace(fmt.Sprintf("Pipeline changed. Reloading file: %s", ei.Path()))
			s.project.pipelines.LoadPipelines()
			s.project.Build(false)
			atomic.StoreInt32(&s.ignoreWrite, 0)
		}

		if s.config.FsWatcher.HotReload {
			s.reloadPage()
		}
	}
}

func (s *Server) HotReload() error {
	s.routing.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Warn(fmt.Sprintf("Upgrade error: %s", err))
		}

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

func (s *Server) reloadPage() {
	message := HotReloadMsg{Reload: true, Url: ""}
	s.sendToClient(message)
}
func (s *Server) sendToClient(msg HotReloadMsg) error {

	s.wsclientsMutex.Lock()
	defer s.wsclientsMutex.Unlock()

	for client := range s.wsclients {
		err := client.WriteJSON(msg)
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
		log.Fatal(fmt.Sprintf("Server Shutdown Failed:%+v", err))
	}
	log.Print("Server exited gracefully")
}
