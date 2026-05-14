package server

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/utils"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type HTTPServer struct {
	httpServer *http.Server
	routing    *http.ServeMux
	config     HTTPServerConfig
}
type HTTPServerConfig struct {
	OutputPath string
	Port       int
}

func NewHTTPServer(cfg HTTPServerConfig) *HTTPServer {
	return &HTTPServer{
		config:  cfg,
		routing: http.NewServeMux(),
	}
}

func (h *HTTPServer) Serve(ctx context.Context) error {
	outputDir := h.config.OutputPath
	fs := http.FileServer(http.Dir(outputDir))

	h.routing.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(r.URL.Path)
		fullPath := filepath.Join(outputDir, path)
		_, err := os.Stat(fullPath)

		if os.IsNotExist(err) {
			if filepath.Ext(path) == "" {
				htmlPath := fullPath + ".html"
				if _, htmlErr := os.Stat(htmlPath); htmlErr == nil {
					r.URL.Path += ".html"
				}
			}
			// TODO add notFound page to config and serve it here
			// w.WriteHeader(http.StatusNotFound)
			// http.ServeFile(w, r, fallbackPath)
			// return
		}

		log.TTrace(log.Server).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("http request")

		fs.ServeHTTP(w, r)
	})

	h.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", h.config.Port),
		Handler:      h.routing,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		_ = h.Shutdown(ctx)
	}()

	log.IInfo(log.Server).
		Int("port", h.config.Port).
		Str("root", h.config.OutputPath).
		Msg("http server starting")

	return h.httpServer.ListenAndServe()
}

func (h *HTTPServer) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var errs []error

	if err := utils.StopComponent(ctx, "http", func(c context.Context) error {
		return h.httpServer.Shutdown(c)
	}); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
