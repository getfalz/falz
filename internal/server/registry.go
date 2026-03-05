package server

import (
	"context"
	"craeft/internal/project"
)

// FileType represents the type of file that changed
type FileType string

const (
	FileTypeContent        FileType = "content"
	FileTypeContentAsset   FileType = "content-asset"
	FileTypeLayout         FileType = "layout"
	FileTypeFragment       FileType = "fragment"
	FileTypeSchemeAsset    FileType = "scheme-asset"
	FileTypePipeline       FileType = "pipeline"
	FileTypeUnknown        FileType = "unknown"
	FileTypeLayoutTemplate FileType = "layout-template"
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
