package server

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/utils"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type HTTPServer struct {
	httpServer *http.Server
	routing    *http.ServeMux
}

func (h *HTTPServer) Serve(ctx context.Context) error {
	fs := http.FileServer(http.Dir(s.config.OutputPath))

	h.routing.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// log.TTrace(log.Server).
		// 	Str("method", r.Method).
		// 	Str("path", r.URL.Path).
		// 	Msg("http request")

		fs.ServeHTTP(w, r)
	})

	h.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      h.routing,
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

	return h.httpServer.ListenAndServe()
}

// TODO remove hotreload, server shutdown logic
func (h *HTTPServer) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var errs []error

	if h.httpServer != nil {
		if err := utils.StopComponent(ctx, "http", func(c context.Context) error {
			return h.httpServer.Shutdown(c)
		}); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
