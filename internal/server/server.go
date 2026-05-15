package server

import (
	"context"
	"errors"
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
	classifier := NewFileClassifier(p, cfg.SourcePath, cfg.SchemePath, cfg.PipesPath, cfg.OutputPath, project.IgnoreList(cfg.Ignore))
	coalescer := NewEventCoalescer()
	handlers := NewHandlers(p)
	handlers.RegisterAll(registry)

	httpSrv := NewHTTPServer(HTTPServerConfig{
		Port:       p.Config.Server.Port,
		OutputPath: p.Config.OutputPath,
	})

	hr := NewHotReload(HotReloadConfig{
		Enabled: cfg.FsWatcher.HotReload,
	}, p, httpSrv.routing)

	debouncer := NewDebouncer(
		DebouncerConfig{
			HotReload: cfg.FsWatcher.HotReload,
		}, p, registry, hr)

	watcher := NewFsWatcher(WatcherConfig{
		SourcePath: cfg.SourcePath,
		Enabled:    cfg.FsWatcher.Enabled,
	}, classifier, coalescer)

	log.DDebug(log.Server).
		Str("out", p.Config.OutputPath).
		Msg("server initialized")

	return &Server{
		project:   p,
		http:      httpSrv,
		hotreload: hr,
		watcher:   watcher,
		debouncer: debouncer,
		registry:  registry,
		handlers:  handlers,
	}

}

func RunServer(c context.Context, p *project.Project) error {
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	if err := p.Build(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.EError(log.Build).Err(err).Msg("initial build failed")
		return err
	}
	log.IInfo(log.Build).Msg("initial build completed")

	s := NewServer(p)

	if s.watcher != nil {
		changes := make(chan FileChange, 256)
		go func() {
			if err := s.watcher.Run(ctx, changes); err != nil && !errors.Is(err, context.Canceled) {
				log.EError(log.Server).Err(err).Msg("file watcher exited with error")
			}
		}()

		go s.debouncer.Run(ctx, changes)
	}

	return s.http.Serve(ctx)
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
