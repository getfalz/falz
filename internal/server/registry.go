package server

import (
	"context"
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

// HandlerFunc is a function adapter for Handler interface
type HandlerFunc func(ctx context.Context, paths []string) error

// EventRegistry maps file types and modification types to handlers
type EventRegistry struct {
	handlers map[FileType]map[ModType]HandlerFunc
}

// NewEventRegistry creates a new event registry
func NewEventRegistry() *EventRegistry {
	return &EventRegistry{
		handlers: make(map[FileType]map[ModType]HandlerFunc),
	}
}

// Register registers a handler for a file type and modification type
func (r *EventRegistry) Register(ft FileType, mt ModType, h HandlerFunc) {
	if r.handlers[ft] == nil {
		r.handlers[ft] = make(map[ModType]HandlerFunc)
	}
	r.handlers[ft][mt] = h
}

// Dispatch dispatches an event to the appropriate handler
func (r *EventRegistry) Dispatch(ctx context.Context, ft FileType, mt ModType, paths []string) error {
	if handlers, ok := r.handlers[ft]; ok {
		if handler, ok := handlers[mt]; ok {
			return handler(ctx, paths)
		}
	}
	return nil
}
