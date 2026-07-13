package pipeline

import (
	"context"
	"strings"
	"testing"
)

func TestNopPipe_Transform_Passthrough(t *testing.T) {
	p := &NopPipe{}
	result, err := p.Transform(context.Background(), []byte("raw bytes"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Content) != "raw bytes" {
		t.Fatalf("got %q, want %q", result.Content, "raw bytes")
	}
}

func TestNopPipe_Hash_IsStable(t *testing.T) {
	p := &NopPipe{}
	first := p.Hash()
	if first == "" {
		t.Fatalf("expected a non-empty hash")
	}
	if second := p.Hash(); second != first {
		t.Fatalf("expected the hash to be stable across calls")
	}
}

func TestMdPipe_Transform_RendersMarkdown(t *testing.T) {
	p := &MdPipe{}
	result, err := p.Transform(context.Background(), []byte("# Hello"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(result.Content), "<h1") {
		t.Fatalf("expected rendered HTML with a heading, got %q", result.Content)
	}
}

func TestTplPipe_Transform_RendersPlushTemplate(t *testing.T) {
	p := TplPipe{}
	result, err := p.Transform(context.Background(), []byte("Hello <%= name %>!"), Vars{"name": "World"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Content) != "Hello World!" {
		t.Fatalf("got %q, want %q", result.Content, "Hello World!")
	}
}

// On a render error, TplPipe.Transform returns the original input
// unmodified alongside the error, rather than dropping the content.
func TestTplPipe_Transform_RenderError_ReturnsOriginalInput(t *testing.T) {
	p := TplPipe{}
	input := []byte("<%= undefinedHelper() %>")
	result, err := p.Transform(context.Background(), input, Vars{})
	if err == nil {
		t.Fatalf("expected an error for an undefined helper call")
	}
	if string(result.Content) != string(input) {
		t.Fatalf("expected the original input to be returned on error, got %q", result.Content)
	}
}

func TestBuiltinPipes_ConfigNamesMatchRegistryKeys(t *testing.T) {
	if (&MdPipe{}).Config().PipelineName != "md" {
		t.Fatalf("expected MdPipe's config name to be %q", "md")
	}
}
