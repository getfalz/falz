package server

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
	"craeft/internal/project"
	"craeft/internal/utils"

	"github.com/gorilla/websocket"
	"github.com/rjeczalik/notify"
)

// FileType represents the type of file that changed
type FileType string

const (
	FileTypeContent      FileType = "content"
	FileTypeContentAsset FileType = "content-asset"
	FileTypeLayout       FileType = "layout"
	FileTypeFragment     FileType = "fragment"
	FileTypeSchemeAsset  FileType = "scheme-asset"
	FileTypePipeline     FileType = "pipeline"
	FileTypeUnknown      FileType = "unknown"
)

// ModType represents the modification type
type ModType string

const (
	ModTypeCreated  ModType = "created"
	ModTypeModified ModType = "modified"
	ModTypeDeleted  ModType = "deleted"
)

// Handler processes file change events
type Handler interface {
	Handle(ctx context.Context, plan *project.Plan, paths []string) error
}

// HandlerFunc is a function adapter for Handler interface
type HandlerFunc func(ctx context.Context, plan *project.Plan, paths []string) error

func (f HandlerFunc) Handle(ctx context.Context, plan *project.Plan, paths []string) error {
	return f(ctx, plan, paths)
}

// EventRegistry maps file types and modification types to handlers
type EventRegistry struct {
	handlers map[FileType]map[ModType]Handler
}

// NewEventRegistry creates a new event registry
func NewEventRegistry() *EventRegistry {
	return &EventRegistry{
		handlers: make(map[FileType]map[ModType]Handler),
	}
}

// Register registers a handler for a file type and modification type
func (r *EventRegistry) Register(ft FileType, mt ModType, h Handler) {
	if r.handlers[ft] == nil {
		r.handlers[ft] = make(map[ModType]Handler)
	}
	r.handlers[ft][mt] = h
}

// Dispatch dispatches an event to the appropriate handler
func (r *EventRegistry) Dispatch(ctx context.Context, plan *project.Plan, ft FileType, mt ModType, paths []string) error {
	if handlers, ok := r.handlers[ft]; ok {
		if handler, ok := handlers[mt]; ok {
			return handler.Handle(ctx, plan, paths)
		}
	}
	return nil
}

// FileClassification represents the result of classifying a file
type FileClassification struct {
	Type FileType
	Mod  ModType
	Path string
}

// FileClassifier determines the type of a file based on its path and project structure
type FileClassifier struct {
	project *project.Project
}

// NewFileClassifier creates a new file classifier
func NewFileClassifier(p *project.Project) *FileClassifier {
	return &FileClassifier{project: p}
}

// Classify determines the file type and modification type for a given path and event
func (fc *FileClassifier) Classify(path string, event notify.Event, root string) FileClassification {
	// Determine modification type
	modType := ModTypeModified
	switch event {
	case notify.Create:
		modType = ModTypeCreated
	case notify.Remove:
		modType = ModTypeDeleted
	case notify.Write:
		modType = ModTypeModified
	}

	var fileType FileType

	switch root {
	case "source":
		fileType = fc.classifySourceFile(path)
	case "scheme":
		fileType = fc.classifySchemeFile(path)
	case "pipes":
		fileType = FileTypePipeline
	default:
		fileType = FileTypeUnknown
	}

	return FileClassification{
		Type: fileType,
		Mod:  modType,
		Path: path,
	}
}

// classifySourceFile determines if a source file is content or asset
func (fc *FileClassifier) classifySourceFile(path string) FileType {
	dir := filepath.Dir(path)

	lmap, err := fc.project.GetLayouts(dir)
	if err != nil {
		return FileTypeUnknown
	}

	for _, lyt := range lmap {
		for _, cc := range lyt.Contents() {
			glob := filepath.ToSlash(filepath.Join(dir, cc.Path))
			matched, err := filepath.Match(glob, path)
			if err == nil && matched {
				return FileTypeContent
			}

			for _, a := range cc.Assets() {
				glob := filepath.ToSlash(filepath.Join(dir, a.Source))
				matched, err := filepath.Match(glob, path)
				if err == nil && matched {
					return FileTypeContentAsset
				}
			}
		}
	}

	return FileTypeUnknown
}

// classifySchemeFile determines if a scheme file is layout, fragment, or asset
func (fc *FileClassifier) classifySchemeFile(path string) FileType {
	if strings.HasSuffix(path, fc.project.Config.Layout) {
		return FileTypeLayout
	}

	dir := filepath.Dir(path)

	lmap, err := fc.project.GetLayouts(dir)
	if err != nil {
		return FileTypeUnknown
	}

	for _, lyt := range lmap {
		for _, f := range lyt.Fragments() {
			// TODO: DEBUG
			fmt.Printf("===Fragments: %s = %s:%s", path, dir, f.Source)
			if path == filepath.ToSlash(filepath.Join(dir, f.Source)) {
				return FileTypeFragment
			}
		}

		for _, a := range lyt.Assets() {
			glob := filepath.ToSlash(filepath.Join(dir, a.Source))
			matched, err := filepath.Match(glob, path)
			if err == nil && matched {
				return FileTypeSchemeAsset
			}
		}
	}

	return FileTypeUnknown
}

func (s *Server) registerHandlers() {
	// Content handlers
	s.eventRegistry.Register(FileTypeContent, ModTypeModified, HandlerFunc(s.handleContentModified))
	s.eventRegistry.Register(FileTypeContent, ModTypeCreated, HandlerFunc(s.handleContentCreated))
	s.eventRegistry.Register(FileTypeContent, ModTypeDeleted, HandlerFunc(s.handleContentDeleted))

	// Content Asset handlers
	s.eventRegistry.Register(FileTypeContentAsset, ModTypeModified, HandlerFunc(s.handleContentAssetModified))
	s.eventRegistry.Register(FileTypeContentAsset, ModTypeCreated, HandlerFunc(s.handleContentAssetCreated))
	s.eventRegistry.Register(FileTypeContentAsset, ModTypeDeleted, HandlerFunc(s.handleContentAssetDeleted))

	// Layout handlers
	s.eventRegistry.Register(FileTypeLayout, ModTypeModified, HandlerFunc(s.handleLayoutModified))
	s.eventRegistry.Register(FileTypeLayout, ModTypeCreated, HandlerFunc(s.handleLayoutCreated))
	s.eventRegistry.Register(FileTypeLayout, ModTypeDeleted, HandlerFunc(s.handleLayoutDeleted))

	// Fragment handlers
	s.eventRegistry.Register(FileTypeFragment, ModTypeModified, HandlerFunc(s.handleFragmentModified))
	s.eventRegistry.Register(FileTypeFragment, ModTypeCreated, HandlerFunc(s.handleFragmentModified))
	s.eventRegistry.Register(FileTypeFragment, ModTypeDeleted, HandlerFunc(s.handleFragmentModified))

	// Scheme Asset handlers
	s.eventRegistry.Register(FileTypeSchemeAsset, ModTypeModified, HandlerFunc(s.handleSchemeAssetModified))
	s.eventRegistry.Register(FileTypeSchemeAsset, ModTypeCreated, HandlerFunc(s.handleSchemeAssetCreated))
	s.eventRegistry.Register(FileTypeSchemeAsset, ModTypeDeleted, HandlerFunc(s.handleSchemeAssetDeleted))

	// Pipeline handlers
	s.eventRegistry.Register(FileTypePipeline, ModTypeModified, HandlerFunc(s.handlePipelineModified))
	s.eventRegistry.Register(FileTypePipeline, ModTypeCreated, HandlerFunc(s.handlePipelineModified))
	s.eventRegistry.Register(FileTypePipeline, ModTypeDeleted, HandlerFunc(s.handlePipelineModified))
}

type Server struct {
	config     *project.CraeftConfig
	project    *project.Project
	httpServer *http.Server
	routing    *http.ServeMux

	upgrader       websocket.Upgrader
	wsclients      map[*websocket.Conn]bool
	wsclientsMutex sync.Mutex

	eventRegistry *EventRegistry
	classifier    *FileClassifier
}

func NewServer(p *project.Project) *Server {
	s := &Server{
		config:        p.Config,
		project:       p,
		routing:       http.NewServeMux(),
		wsclients:     make(map[*websocket.Conn]bool),
		classifier:    NewFileClassifier(p),
		eventRegistry: NewEventRegistry(),
	}

	s.registerHandlers()

	log.DDebug(log.Server).
		Str("out", p.Config.OutputPath).
		Msg("server initialized")

	return s
}

func RunServer(c context.Context, p *project.Project) error {
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	// TODO: use project options
	opt := project.BuildOptions{
		Concurrency: runtime.NumCPU(),
		UseCAS:      true,
		CleanStale:  true,
	}

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
