package js

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTestHTTPVM starts an httptest.Server, returns a VM whose http module is
// allowed to talk to it, and the server itself (caller must t.Cleanup close
// it or rely on httptest.Server's own cleanup via t.Cleanup here).
func newTestHTTPVM(t *testing.T, handler http.HandlerFunc) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return srv, u.Host
}

func TestHTTP_Get_ReturnsResponseWithStatusAndBody(t *testing.T) {
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	})
	_ = srv

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{host}}, NewSharedTransport(0))
	result, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			const res = http.get(url);
			return res.status + ':' + res.ok + ':' + res.text();
		}`, "run", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "200:true:hello" {
		t.Fatalf("got %v, want %q", result.Export(), "200:true:hello")
	}
}

func TestHTTP_Get_JSONHelper(t *testing.T) {
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"name": "acme"})
	})

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{host}}, NewSharedTransport(0))
	result, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			return http.get(url).json().name;
		}`, "run", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "acme" {
		t.Fatalf("got %v, want %q", result.Export(), "acme")
	}
}

func TestHTTP_Post_ObjectBody_AutoEncodedAsJSON(t *testing.T) {
	var gotContentType string
	var gotBody string
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
	})

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{host}}, NewSharedTransport(0))
	result, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			return http.post(url, { name: "alice" }).status;
		}`, "run", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToInteger() != 201 {
		t.Fatalf("got status %v, want 201", result.Export())
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Fatalf("expected an auto-encoded JSON content type, got %q", gotContentType)
	}
	if !strings.Contains(gotBody, `"name":"alice"`) {
		t.Fatalf("expected JSON-encoded body, got %q", gotBody)
	}
}

func TestHTTP_Post_StringBody_PlainTextContentType(t *testing.T) {
	var gotContentType string
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	})

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{host}}, NewSharedTransport(0))
	_, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			return http.post(url, "plain text").status;
		}`, "run", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(gotContentType, "text/plain") {
		t.Fatalf("expected text/plain content type for a string body, got %q", gotContentType)
	}
}

func TestHTTP_NonOKStatus_ThrowsWithStatusAndResponse(t *testing.T) {
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	})

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{host}}, NewSharedTransport(0))
	result, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			try {
				http.get(url);
				return "no error thrown";
			} catch (e) {
				return e.status + ':' + e.response.status + ':' + e.response.text();
			}
		}`, "run", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "404:404:not found" {
		t.Fatalf("got %v, want %q", result.Export(), "404:404:not found")
	}
}

func TestHTTP_DisallowedHost_ThrowsBeforeRequest(t *testing.T) {
	requested := false
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		requested = true
		w.WriteHeader(http.StatusOK)
	})
	_ = host // deliberately NOT added to AllowedHosts

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{"other.example.com"}}, NewSharedTransport(0))
	_, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			return http.get(url).status;
		}`, "run", srv.URL)
	if err == nil {
		t.Fatalf("expected an error for a disallowed host")
	}
	if requested {
		t.Fatalf("expected the request to be rejected before ever reaching the server")
	}
}

func TestHTTP_NoAllowedHosts_DisablesOutboundHTTP(t *testing.T) {
	srv, _ := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	vm := newTestVM(t, nil, HTTPPolicy{}, NewSharedTransport(0))
	_, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			return http.get(url).status;
		}`, "run", srv.URL)
	if err == nil {
		t.Fatalf("expected outbound HTTP to be disabled with an empty AllowedHosts list")
	}
}

func TestHTTP_MissingURL_ThrowsTypeError(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{"*"}}, NewSharedTransport(0))
	_, err := runJSFunc(t, vm, `
		function run() {
			const http = require('http');
			return http.get();
		}`, "run")
	if err == nil {
		t.Fatalf("expected an error when url is missing")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("expected a TypeError, got %v", err)
	}
}

func TestHTTP_Head_ResponseBodyIsEmpty(t *testing.T) {
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected a HEAD request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{host}}, NewSharedTransport(0))
	result, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			return http.head(url).text();
		}`, "run", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Export() != "" {
		t.Fatalf("expected an empty body for a HEAD request, got %q", result.Export())
	}
}

func TestHTTP_MaxBodyBytes_CapsResponseBody(t *testing.T) {
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 100)))
	})

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{host}, MaxBodyBytes: 10}, NewSharedTransport(0))
	_, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			return http.get(url).text();
		}`, "run", srv.URL)
	if err == nil {
		t.Fatalf("expected an error when the response exceeds MaxBodyBytes")
	}
}

func TestHTTP_CustomHeaders_SentToServer(t *testing.T) {
	var gotHeader string
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	})

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{host}}, NewSharedTransport(0))
	_, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			return http.get(url, { "X-Custom": "hi" }).status;
		}`, "run", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotHeader != "hi" {
		t.Fatalf("expected the custom header to reach the server, got %q", gotHeader)
	}
}

func TestHTTP_Headers_ConstructorAndMethods(t *testing.T) {
	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{"*"}}, NewSharedTransport(0))
	result, err := runJSFunc(t, vm, `
		function run() {
			const http = require('http');
			const h = new http.Headers({ "Content-Type": "text/plain" });
			h.append("X-Multi", "a");
			h.append("X-Multi", "b");
			h.set("X-Single", "one");
			const parts = [
				h.get("Content-Type"),
				h.has("Content-Type"),
				h.has("Nope"),
				h.get("X-Multi"),
				h.get("X-Single"),
			];
			h.delete("X-Single");
			parts.push(h.has("X-Single"));
			return parts.join(',');
		}`, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "text/plain,true,false,a, b,one,false"
	if result.Export() != want {
		t.Fatalf("got %v, want %q", result.Export(), want)
	}
}

func TestHTTP_Headers_UsableAsRequestHeaders(t *testing.T) {
	var gotHeader string
	srv, host := newTestHTTPVM(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-From-Headers-Object")
		w.WriteHeader(http.StatusOK)
	})

	vm := newTestVM(t, nil, HTTPPolicy{AllowedHosts: []string{host}}, NewSharedTransport(0))
	_, err := runJSFunc(t, vm, `
		function run(url) {
			const http = require('http');
			const h = new http.Headers();
			h.set("X-From-Headers-Object", "yes");
			return http.get(url, h).status;
		}`, "run", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotHeader != "yes" {
		t.Fatalf("expected the Headers instance to be usable directly as the headers argument, got %q", gotHeader)
	}
}
