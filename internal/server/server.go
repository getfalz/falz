package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"craeft/internal/log"
	"craeft/internal/project"
)

type Server struct {
	project   *project.Project
	http      *HTTPServer
	hotreload *HotReload
	watcher   *FSWatcher
	debouncer *Debouncer
	registry  *EventRegistry
	handlers  *Handlers
}

func NewServer(p *project.Project) *Server {
	cfg := p.Config

	registry := NewEventRegistry()
	classifier := NewFileClassifier(p, cfg.SourcePath, cfg.SchemePath, cfg.PipesPath, cfg.OutputPath, p.Ignore)
	coalescer := NewEventCoalescer()
	handlers := NewHandlers(p)
	handlers.RegisterAll(registry)

	watcher := NewFsWatcher(cfg.SourcePath, coalescer)

	s := &Server{
		project:  p,
		watcher:  watcher,
		registry: registry,
		handlers: handlers,
	}

	hotReload := cfg.Server.HotReload
	if hotReload && !cfg.Server.Enable {
		log.WWarn(log.Server).
			Msg("hot reload requested but HTTP server is disabled; ignoring (enable with --start-server)")
		hotReload = false
	}

	if cfg.Server.Enable {
		s.http = NewHTTPServer(HTTPServerConfig{
			Port:       p.Config.Server.Port,
			OutputPath: p.Config.OutputPath,
		})

		if hotReload {
			s.hotreload = NewHotReload(p, s.http.routing)
		}
	}

	var rl reloader
	if s.hotreload != nil {
		rl = s.hotreload
	}
	debouncer := NewDebouncer(
		DebouncerConfig{
			HotReload: hotReload,
		}, p, registry, classifier, rl)

	s.debouncer = debouncer

	log.DDebug(log.Server).
		Str("out", p.Config.OutputPath).
		Msg("server initialized")

	return s
}

func RunServer(c context.Context, p *project.Project, forcePort bool) error {
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	if err := p.Build(ctx); errors.Is(err, context.Canceled) {
		return err
	}
	log.IInfo(log.Build).Msg("initial build completed")

	if err := p.Build(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		log.EError(log.Build).Err(err).Msg("initial build completed with errors")
	} else {
		log.IInfo(log.Build).Msg("initial build completed")
	}

	var ln net.Listener
	if p.Config.Server.Enable {
		retries := portRetries
		if forcePort {
			retries = 0
		}
		l, port, err := listenSearchPort(p.Config.Server.Host, p.Config.Server.Port, retries)
		if err != nil {
			return err
		}
		ln = l
		p.Config.Server.Port = port
	}

	s := NewServer(p)

	changes := make(chan RawChange, 256)
	go func() {
		if err := s.watcher.Run(ctx, changes); err != nil && !errors.Is(err, context.Canceled) {
			log.EError(log.Server).Err(err).Msg("file watcher exited with error")
		}
	}()

	go s.debouncer.Run(ctx, changes)

	if !p.Config.Server.Enable {
		<-ctx.Done()
		return nil
	}

	if p.Config.Server.OpenBrowser {
		url := fmt.Sprintf("http://%s:%d/", p.Config.Server.Host, p.Config.Server.Port)
		go tryOpenBrowser(ctx, url)
	}

	if err := s.http.ServeOn(ctx, ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var errs []error
	log.IInfo(log.Server).Msg("shutdown begin")

	if err := s.http.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}

	if s.hotreload != nil {
		if err := s.hotreload.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
