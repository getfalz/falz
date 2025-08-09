package project

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"craeft/internal/executor"
	"craeft/internal/log"

	"github.com/gorilla/websocket"
	"github.com/rjeczalik/notify"
)

type Server struct {
	config     *CraeftConfig
	project    *Project
	httpServer *http.Server
	routing    *http.ServeMux

	upgrader       websocket.Upgrader
	wsclients      map[*websocket.Conn]bool
	wsclientsMutex sync.Mutex

	pool *executor.PoolExecutor
}

func NewServer(p *Project, pool *executor.PoolExecutor) *Server {

	s := &Server{
		config:  p.Config,
		project: p,
		routing: http.NewServeMux(),

		wsclients: make(map[*websocket.Conn]bool),

		pool: pool,
	}

	return s
}

func RunServer(c context.Context, p *Project) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//TODO set with args
	concurrency := runtime.GOMAXPROCS(0)
	pool := executor.NewPoolExecutor(concurrency, 1024, executor.NewTimerDebouncer(120*time.Millisecond))
	pool.Start(ctx)
	defer pool.Stop()

	opt := BuildOptions{UseCache: false, Concurrency: runtime.NumCPU()}
	if _, err := p.Build(ctx, opt); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	s := NewServer(p, pool)
	if p.Config.FsWatcher.HotReload {
		_ = s.HotReload()
	}
	if p.Config.FsWatcher.Content {
		go func() { _ = s.WatchFiles(ctx) }()
	}
	if p.Config.FsWatcher.Theme {
		go func() { _ = s.WatchTheme(ctx) }()
	}
	if p.Config.FsWatcher.Pipes {
		go func() { _ = s.WatchPipes(ctx) }()
	}
	return s.ServeFiles(ctx)
}

func (s *Server) ServeFiles(ctx context.Context) error {
	fs := http.FileServer(http.Dir(s.config.OutputPath))

	s.routing.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Info(fmt.Sprintf("Requested: %s", r.URL.Path))

		// if _, err := os.Stat(s.config.OutputPath + r.URL.Path); os.IsNotExist(err) {
		// 	http.NotFound(w, r)
		// 	return
		// }

		// w.Header().Add("Cache-Control", "max-age=604800")
		fs.ServeHTTP(w, r)
	})

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      s.routing,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		_ = s.httpServer.Shutdown(context.Background())
	}()

	log.Info(fmt.Sprintf("Server started at port %d", s.config.Server.Port))

	return s.httpServer.ListenAndServe()
}

type HotReloadMsg struct {
	Reload bool   `json:"reload"`
	URL    string `json:"url"`
}

func (s *Server) HotReload() error {
	// s.upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	s.routing.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Warn(fmt.Sprintf("Upgrade error: %s", err))
		}

		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		})

		s.wsclientsMutex.Lock()
		s.wsclients[conn] = true
		s.wsclientsMutex.Unlock()

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
					// break
					return
				}
			}
		}()

		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for range t.C {
				s.wsclientsMutex.Lock()
				if _, ok := s.wsclients[conn]; !ok {
					s.wsclientsMutex.Unlock()
					return
				}
				s.wsclientsMutex.Unlock()
				_ = conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
			}
		}()

	})

	return nil
}

func (s *Server) sendToClient(msg HotReloadMsg) {
	s.wsclientsMutex.Lock()
	clients := make([]*websocket.Conn, 0, len(s.wsclients))
	for c := range s.wsclients {
		clients = append(clients, c)
	}
	s.wsclientsMutex.Unlock()

	for _, c := range clients {
		if err := c.WriteJSON(msg); err != nil {
			_ = c.Close()
			s.wsclientsMutex.Lock()
			delete(s.wsclients, c)
			s.wsclientsMutex.Unlock()
		}
	}
}

func (s *Server) reloadPage() {
	message := HotReloadMsg{Reload: true, URL: ""}
	s.sendToClient(message)
}

// WatchFiles watches for changes in the source directory and triggers a rebuild
// what it don't watch is changes on the config file. it also only watch for file changes
// be careful, some files may end into multiple files, in this case, the websocket is notified with the last build file
func (s *Server) WatchFiles(ctx context.Context) error {
	events := make(chan notify.EventInfo, 128)

	absPath, err := filepath.Abs(s.config.SourcePath)
	if err != nil {
		return err
	}

	if err := notify.Watch(absPath+"/...", events, notify.Write); err != nil {
		return fmt.Errorf("can't watch files for change: %w", err)
	}
	defer notify.Stop(events)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ei := <-events:
			if ei.Event() != notify.Write {
				continue
			}
			relPath, err := filepath.Rel(absPath, ei.Path())
			if err != nil {
				log.Warn(fmt.Sprintf("Can't resolve relative path: %s", err))
				continue
			}

			if s.project.ignoreFile(relPath) {
				continue
			}

			log.Trace(fmt.Sprintf("File changed. UPDATE: %s", relPath))

			s.pool.Submit(rebuildTask{s: s, rel: relPath})

		}
	}
}

func (s *Server) WatchTheme(ctx context.Context) error {
	events := make(chan notify.EventInfo, 128)

	absScheme, err := filepath.Abs(s.config.SchemePath)
	if err != nil {
		return err
	}

	if err := notify.Watch(absScheme+"/...", events, notify.Write); err != nil {
		return fmt.Errorf("watch theme: %w", err)
	}
	defer notify.Stop(events)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ei := <-events:
			if ei.Event() != notify.Write {
				continue
			}
			rel, err := filepath.Rel(absScheme, ei.Path())
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(rel)

			keys := s.project.Layouts().InvalidateAndGetAffected(rel)
			paths := s.project.AffectedByLayoutKeys(keys)

			_, err = s.project.plan()
			if err != nil {
				log.Warn(fmt.Sprintf("rebuild plan: %s", err))
				continue
			}

			for _, pth := range paths {
				s.pool.Submit(rebuildTask{s: s, rel: pth})
			}
			if s.config.FsWatcher.HotReload && len(paths) == 0 {
				s.reloadPage()
			}
		}
	}
}

func (s *Server) WatchPipes(ctx context.Context) error {
	events := make(chan notify.EventInfo, 128)

	absPipes, err := filepath.Abs(s.config.PipesPath)
	if err != nil {
		return err
	}

	// TODO: start a watcher for each pipe folder. this way, we can:
	// * selectively watch for pipes in ~/.config and .craeft/pipes
	// * when a pipe change, we can use the pipe information to just rebuild files transformed by the pipe
	if err := notify.Watch(absPipes+"/...", events, notify.Write); err != nil {
		return fmt.Errorf("watch pipes: %w", err)
	}
	defer notify.Stop(events)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ei := <-events:
			if ei.Event() != notify.Write {
				continue
			}
			rel, err := filepath.Rel(absPipes, ei.Path())
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(rel)
			parts := strings.Split(rel, "/")
			if len(parts) == 0 || parts[0] == "" {
				continue
			}
			pipeName := parts[0]

			if err := s.project.Pipelines().Reload(pipeName); err != nil {
				log.Warn(fmt.Sprintf("reload pipeline %s: %v", pipeName, err))
			}

			paths := s.project.AffectedByPipeline(pipeName)
			for _, pth := range paths {
				s.pool.Submit(rebuildTask{s: s, rel: pth})
			}
			if s.config.FsWatcher.HotReload && len(paths) == 0 {
				s.reloadPage()
			}
		}
	}
}

func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Fatal(fmt.Sprintf("Server Shutdown Failed:%+v", err))
	}
	log.Print("Server exited gracefully")
}
