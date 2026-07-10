package server

import (
	"context"
	"errors"
	"testing"
)

func TestEventRegistry_Dispatch_CallsRegisteredHandler(t *testing.T) {
	r := NewEventRegistry()

	var got []string
	r.Register(FileTypeContent, ModTypeModified, func(ctx context.Context, paths []string) error {
		got = paths
		return nil
	})

	if err := r.Dispatch(context.Background(), FileTypeContent, ModTypeModified, []string{"about.md"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(got) != 1 || got[0] != "about.md" {
		t.Fatalf("expected the handler to receive the dispatched paths, got %v", got)
	}
}

func TestEventRegistry_Dispatch_UnregisteredCombo_IsNoop(t *testing.T) {
	r := NewEventRegistry()
	r.Register(FileTypeContent, ModTypeModified, func(ctx context.Context, paths []string) error {
		t.Fatalf("handler should not be called for a different (type, mod) combination")
		return nil
	})

	// Same type, different mod.
	if err := r.Dispatch(context.Background(), FileTypeContent, ModTypeDeleted, []string{"about.md"}); err != nil {
		t.Fatalf("expected no error for an unregistered combination, got %v", err)
	}
	// Different type entirely.
	if err := r.Dispatch(context.Background(), FileTypeLayout, ModTypeModified, []string{"layout.xml"}); err != nil {
		t.Fatalf("expected no error for an unregistered type, got %v", err)
	}
}

func TestEventRegistry_Dispatch_PropagatesHandlerError(t *testing.T) {
	r := NewEventRegistry()
	wantErr := errors.New("boom")
	r.Register(FileTypeContent, ModTypeModified, func(ctx context.Context, paths []string) error {
		return wantErr
	})

	if err := r.Dispatch(context.Background(), FileTypeContent, ModTypeModified, []string{"about.md"}); !errors.Is(err, wantErr) {
		t.Fatalf("expected the handler's error to propagate, got %v", err)
	}
}

func TestEventRegistry_Register_LaterCallOverwritesHandler(t *testing.T) {
	r := NewEventRegistry()

	called := ""
	r.Register(FileTypeContent, ModTypeModified, func(ctx context.Context, paths []string) error {
		called = "first"
		return nil
	})
	r.Register(FileTypeContent, ModTypeModified, func(ctx context.Context, paths []string) error {
		called = "second"
		return nil
	})

	if err := r.Dispatch(context.Background(), FileTypeContent, ModTypeModified, nil); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if called != "second" {
		t.Fatalf("expected the later Register call to win, got %q", called)
	}
}

// TestEventRegistry_RegisterAll_CoversEveryDispatchedCombination is a
// real-usage smoke test: it registers all of Handlers' real handlers via
// RegisterAll and checks that every (FileType, ModType) combination the
// debouncer can actually classify a file into has a handler wired up, so a
// real file-save event is never silently dropped.
func TestEventRegistry_RegisterAll_CoversEveryDispatchedCombination(t *testing.T) {
	p, _ := newTestServerProject(t)
	h := NewHandlers(p)

	r := NewEventRegistry()
	h.RegisterAll(r)

	combos := []struct {
		ft FileType
		mt ModType
	}{
		{FileTypeContent, ModTypeCreated},
		{FileTypeContent, ModTypeModified},
		{FileTypeContent, ModTypeDeleted},
		{FileTypeContentAsset, ModTypeCreated},
		{FileTypeContentAsset, ModTypeModified},
		{FileTypeContentAsset, ModTypeDeleted},
		{FileTypeLayout, ModTypeCreated},
		{FileTypeLayout, ModTypeModified},
		{FileTypeLayout, ModTypeDeleted},
		{FileTypeFragment, ModTypeCreated},
		{FileTypeFragment, ModTypeModified},
		{FileTypeFragment, ModTypeDeleted},
		{FileTypeLayoutTemplate, ModTypeCreated},
		{FileTypeLayoutTemplate, ModTypeModified},
		{FileTypeLayoutTemplate, ModTypeDeleted},
		{FileTypeSchemeAsset, ModTypeCreated},
		{FileTypeSchemeAsset, ModTypeModified},
		{FileTypeSchemeAsset, ModTypeDeleted},
		{FileTypePipeline, ModTypeCreated},
		{FileTypePipeline, ModTypeModified},
		{FileTypePipeline, ModTypeDeleted},
		{FileTypeConfig, ModTypeModified},
	}

	for _, c := range combos {
		if _, ok := r.handlers[c.ft]; !ok {
			t.Errorf("no handlers registered at all for FileType %q", c.ft)
			continue
		}
		if _, ok := r.handlers[c.ft][c.mt]; !ok {
			t.Errorf("missing handler for (%s, %s)", c.ft, c.mt)
		}
	}
}
