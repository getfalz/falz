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

	"craeft/internal/log"
	"craeft/internal/utils"

	"github.com/gorilla/websocket"
	"github.com/rjeczalik/notify"
)

// TODO refactor
type Server struct {
	config     *CraeftConfig
	project    *Project
	httpServer *http.Server
	routing    *http.ServeMux

	upgrader       websocket.Upgrader
	wsclients      map[*websocket.Conn]bool
	wsclientsMutex sync.Mutex

	// pool *executor.PoolExecutor
}

// func NewServer(p *Project, pool *executor.PoolExecutor) *Server {
func NewServer(p *Project) *Server {

	s := &Server{
		config:  p.Config,
		project: p,
		routing: http.NewServeMux(),

		wsclients: make(map[*websocket.Conn]bool),

		// pool: pool,
	}

	log.DDebug(log.Server).
		// Int("concurrency", pool.Concurrency).
		Str("out", p.Config.OutputPath).
		Msg("server initialized")

	return s
}

func RunServer(c context.Context, p *Project) error {
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	//TODO set with args
	// concurrency := runtime.GOMAXPROCS(0)
	// pool := executor.NewPoolExecutor(concurrency, 1024, executor.NewTimerDebouncer(120*time.Millisecond))
	// pool.Start(ctx)
	// defer func() {
	// 	_ = utils.StopComponent(ctx, "worker-pool", func(context.Context) error {
	// 		pool.Stop()
	// 		return nil
	// 	})
	// }()

	opt := BuildOptions{Concurrency: runtime.NumCPU()}
	stats, err := p.Build(ctx, opt)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.EError(log.Build).
			Err(err).
			Str("stats", stats.String()).
			Msg("initial build failed")
		return err
	}

	log.IInfo(log.Build).
		Str("stats", stats.String()).
		Msg("initial build ok")

	s := NewServer(p)
	if p.Config.FsWatcher.HotReload {
		if err := s.HotReload(); err != nil {
			log.EError(log.Server).Err(err).Msg("hot-reload setup failed")
		} else {
			log.IInfo(log.Server).Msg("hot-reload enabled")
		}
	}
	if p.Config.FsWatcher.Content {
		go func() {
			if err := s.WatchFiles(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.EError(log.Server).Err(err).Msg("content watcher exited with error")
			}
		}()
	}
	if p.Config.FsWatcher.Theme {
		go func() {
			if err := s.WatchTheme(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.EError(log.Server).Err(err).Msg("theme watcher exited with error")
			}
		}()
	}
	if p.Config.FsWatcher.Pipes {
		go func() {
			if err := s.WatchPipes(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.EError(log.Server).Err(err).Msg("pipes watcher exited with error")
			}
		}()
	}
	return s.ServeFiles(ctx)
}

func (s *Server) ServeFiles(ctx context.Context) error {
	fs := http.FileServer(http.Dir(s.config.OutputPath))

	s.routing.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.TTrace(log.Server).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("http request")

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
		_ = s.Shutdown(ctx)
	}()

	log.IInfo(log.Server).
		Int("port", s.config.Server.Port).
		Str("root", s.config.OutputPath).
		Msg("http server starting")

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
			log.DDebug(log.Server).Err(err).Msg("websocket upgrade failed")
			return
		}

		log.IInfo(log.Server).Str("remote", r.RemoteAddr).Msg("websocket connected")

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
				log.DDebug(log.Server).Msg("websocket disconnected")
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
			defer conn.Close()
			for range t.C {
				s.wsclientsMutex.Lock()
				if _, ok := s.wsclients[conn]; !ok {
					s.wsclientsMutex.Unlock()
					return
				}
				s.wsclientsMutex.Unlock()
				_ = conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
				log.TTrace(log.Server).Msg("websocket ping")
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

	log.DDebug(log.Server).
		Int("clients", len(clients)).
		Bool("reload", msg.Reload).
		Str("url", msg.URL).
		Msg("broadcast hot-reload")

	for _, c := range clients {
		if err := c.WriteJSON(msg); err != nil {
			_ = c.Close()
			s.wsclientsMutex.Lock()
			delete(s.wsclients, c)
			s.wsclientsMutex.Unlock()
			log.DDebug(log.Server).Err(err).Msg("websocket write failed; client removed")
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
	log.IInfo(log.Server).Str("dir", absPath).Msg("watching content changes")

	defer func() {
		_ = utils.StopComponent(ctx, "source-watcher", func(context.Context) error {
			notify.Stop(events)
			return nil
		})
	}()

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
				log.DDebug(log.Server).Err(err).Msg("cannot resolve relative path")
				continue
			}

			if s.project.ignoreFile(relPath) {
				continue
			}

			log.TTrace(log.Server).Str("rel", filepath.ToSlash(relPath)).Msg("file changed")

			// s.pool.Submit(rebuildTask{s: s, rel: relPath})

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
	log.IInfo(log.Server).Str("dir", absScheme).Msg("watching theme changes")
	defer func() {
		_ = utils.StopComponent(ctx, "scheme-watcher", func(context.Context) error {
			notify.Stop(events)
			return nil
		})
	}()

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

			log.TTrace(log.Server).Str("rel", rel).Msg("theme changed")

			// keys := s.project.Layouts().InvalidateAndGetAffected(rel)
			// paths := s.project.AffectedByLayoutKeys(keys)
			//
			// _, err = s.project.Plan()
			// if err != nil {
			// 	log.DDebug(log.Server).Err(err).Msg("rebuild plan failed after theme change")
			// 	continue
			// }
			//
			// for _, pth := range paths {
			// 	s.pool.Submit(rebuildTask{s: s, rel: pth})
			// }
			// if s.config.FsWatcher.HotReload && len(paths) == 0 {
			// 	s.reloadPage()
			// }
		}
	}
}

func (s *Server) WatchPipes(ctx context.Context) error {
	events := make(chan notify.EventInfo, 128)

	absPipes, err := filepath.Abs(s.config.PipesPath)
	if err != nil {
		return err
	}

	if err := notify.Watch(absPipes+"/...", events, notify.Write); err != nil {
		return fmt.Errorf("watch pipes: %w", err)
	}
	log.IInfo(log.Server).Str("dir", absPipes).Msg("watching pipeline changes")
	defer func() {
		_ = utils.StopComponent(ctx, "pipe-watcher", func(context.Context) error {
			notify.Stop(events)
			return nil
		})
	}()

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

			if !s.project.pipelines.HasPipeline(pipeName) {
				continue
			}

			// if err := s.project.Pipelines().Reload(pipeName); err != nil {
			// 	log.DDebug(log.Server).Err(err).Str("pipe", pipeName).Msg("pipeline reload failed")
			// }
			//
			// paths := s.project.AffectedByPipeline(pipeName)
			// for _, pth := range paths {
			// 	s.pool.Submit(rebuildTask{s: s, rel: pth})
			// }
			// if s.config.FsWatcher.HotReload && len(paths) == 0 {
			// 	s.reloadPage()
			// }
		}
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var errs []error
	log.IInfo(log.Server).Msg("shutdown begin")

	if s.httpServer != nil {
		if err := utils.StopComponent(ctx, "http", func(c context.Context) error {
			return s.httpServer.Shutdown(c)
		}); err != nil {
			errs = append(errs, err)
		}
	}

	if err := utils.StopComponent(ctx, "hot-reload", func(context.Context) error {
		s.wsclientsMutex.Lock()
		clients := make([]*websocket.Conn, 0, len(s.wsclients))
		for c := range s.wsclients {
			clients = append(clients, c)
		}
		s.wsclients = make(map[*websocket.Conn]bool)
		s.wsclientsMutex.Unlock()
		for _, c := range clients {
			_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutting down"), time.Now().Add(2*time.Second))
			_ = c.Close()
		}
		return nil
	}); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
