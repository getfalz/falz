package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"craeft/internal/log"
	"craeft/internal/project"
	"craeft/internal/utils"

	"github.com/gorilla/websocket"
)

type Server struct {
	config  *project.CraeftConfig
	project *project.Project

	eventRegistry *EventRegistry
	classifier    *FileClassifier

	httpServer *HTTPServer
	watcher    *FSWatcher
	hotreload  *HotReload
	debouncer  *Debouncer
	handlers   *Handlers
}

func NewServer(p *project.Project) *Server {

	httpServer := NewHttpServer()
	registry := NewEventRegistry()
	handlers.RegisterAll(registry)
	
	s := &Server{
		config:        p.Config,
		project:       p,
		routing:       http.NewServeMux(),
		wsclients:     make(map[*websocket.Conn]bool),
		classifier:    NewFileClassifier(p),
		eventRegistry: registry,
	}

	s.registerHandlers()

	log.DDebug(log.Server).
		Str("out", p.Config.OutputPath).
		Msg("server initialized")

	return s
}

func RunServer(c context.Context, p *project.Project) error {
	// | `RunServer()` | Initial build → `NewServer()` → `watcher.Run()` → `http.Serve()` |
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	err := p.Build(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {

		var be *project.BuildError
		if errors.As(err, &be) {
			log.EError(log.Build).
				Err(err).
				Msg("initial build completed with build errors (continuing)")
		} else {
			log.EError(log.Build).
				Err(err).
				Msg("initial build failed")
			return err
		}
	}

	log.IInfo(log.Build).
		Msg("initial build completed")

	s := NewServer(p)

	if p.Config.FsWatcher.HotReload {
		if err := s.HotReload(); err != nil {
			log.EError(log.Server).Err(err).Msg("hot-reload setup failed")
		} else {
			log.IInfo(log.Server).Msg("hot-reload enabled")
		}
	}

	if p.Config.FsWatcher.Enabled {
		go func() {
			if err := s.Watch(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.EError(log.Server).Err(err).Msg("file watcher exited with error")
			}
		}()
	}

	return s.ServeFiles(ctx)
}

// TODO remove hotreload, httpserver shutdown logic
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
