package js

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
)

// HTTPPolicy is the runtime configuration for the http module
type HTTPPolicy struct {
	// Transport is the shared connection pool for this pipeline.
	Transport *http.Transport

	// Timeout is the per-request deadline (dial + TLS + send + full body).
	// 0 = no client-level deadline (context cancellation still applies).
	Timeout time.Duration

	// AllowedHosts is the allowlist of hosts this pipeline may contact.
	// Entries: "api.example.com", "api.example.com:8080", "*.example.com".
	// Empty = no outbound HTTP permitted
	AllowedHosts []string

	// MaxRedirects caps the redirect chain per request.
	// 0 = no limit. Positive = exact cap.
	MaxRedirects int

	// MaxBodyBytes caps each individual response body. 0 = no cap.
	MaxBodyBytes int64

	// MaxBytesPerTransform caps total response bytes across all http.*
	// calls in one transform invocation. 0 = no cumulative cap.
	MaxBytesPerTransform int64
}

// NewSharedTransport builds the *http.Transport shared by all VMs within a
// single pipeline
func NewSharedTransport(maxConnsPerHost int) *http.Transport {
	if maxConnsPerHost < 0 {
		maxConnsPerHost = 0 // stdlib: 0 = unlimited
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   maxConnsPerHost,
		MaxConnsPerHost:       maxConnsPerHost,
		IdleConnTimeout:       10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// newTransformClient builds a fresh *http.Client for one transform call.
func newTransformClient(policy HTTPPolicy, transport *http.Transport) *http.Client {
	var rt http.RoundTripper = transport
	if policy.MaxBytesPerTransform > 0 {
		budget := &atomic.Int64{}
		budget.Store(policy.MaxBytesPerTransform)
		rt = &cappedTransport{wrapped: transport, budget: budget}
	}
	return &http.Client{
		Transport:     rt,
		Timeout:       policy.Timeout,
		CheckRedirect: buildCheckRedirect(policy),
	}
}

// buildCheckRedirect returns the redirect policy for this pipeline.
func buildCheckRedirect(policy HTTPPolicy) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if policy.MaxRedirects > 0 && len(via) >= policy.MaxRedirects {
			return fmt.Errorf("http: stopped after %d redirects", policy.MaxRedirects)
		}
		return checkAllowedHost(req.URL.Host, policy.AllowedHosts)
	}
}

// cappedTransport enforces a cumulative byte budget across all response bodies
// read in one transform call
type cappedTransport struct {
	wrapped *http.Transport
	budget  *atomic.Int64 // starts at MaxBytesPerTransform, decremented as bytes are read
}

func (t *cappedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.wrapped.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &budgetBody{ReadCloser: resp.Body, budget: t.budget}
	return resp, nil
}

// budgetBody wraps a response body and decrements the shared transform budget.
// All requests in the same transform share this counter via the cappedTransport.
type budgetBody struct {
	io.ReadCloser
	budget *atomic.Int64
}

func (b *budgetBody) Read(p []byte) (n int, err error) {
	n, err = b.ReadCloser.Read(p)
	if n > 0 {
		if remaining := b.budget.Add(-int64(n)); remaining < 0 {
			return n, fmt.Errorf("cumulative response size exceeds per-transform limit")
		}
	}
	return
}

// checkAllowedHost returns nil if host is in allowedHosts, or an error.
//
// host is the URL host field — "hostname" or "hostname:port".
// Allowed entries:
//   - "api.example.com"       — any port on that hostname
//   - "api.example.com:8080"  — exactly that hostname:port
//   - "*.example.com"         — any subdomain, any port
//
// Empty allowedHosts = no hosts permitted
func checkAllowedHost(host string, allowedHosts []string) error {
	if len(allowedHosts) == 0 {
		return fmt.Errorf("no allowedHosts configured — outbound HTTP is disabled for this pipeline")
	}

	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		hostname, port = host, ""
	}

	for _, allowed := range allowedHosts {
		allowedHost, allowedPort, aerr := net.SplitHostPort(allowed)
		if aerr != nil {
			allowedHost, allowedPort = allowed, ""
		}

		hostnameMatch := hostname == allowedHost ||
			(strings.HasPrefix(allowedHost, "*.") &&
				strings.HasSuffix(hostname, allowedHost[1:]) &&
				len(hostname) > len(allowedHost[1:]))

		portMatch := allowedPort == "" || allowedPort == port

		if hostnameMatch && portMatch {
			return nil
		}
	}
	return fmt.Errorf("host %q is not in the pipeline's allowedHosts list", host)
}

// httpModule is the per-module environment closed over by every JS-callable
// function
type httpModule struct {
	vm           *goja.Runtime
	ctx          context.Context
	maxBytes     int64
	allowedHosts []string
	client       *http.Client
}

// @ts namespace http
//
// @doc
// Synchronous HTTP client for pipeline scripts.
//
// Every method executes the request synchronously and returns a `Response`
// directly. Requests are cancelled automatically when
// the pipeline context times out or is cancelled.
//
// All methods throw on non-2xx responses. The thrown error has two extra
// properties:
//   - `status`   — the HTTP status code (number)
//   - `response` — the full `Response` object so you can inspect headers and
//     body on error
//
// Body encoding for POST / PUT / PATCH / DELETE:
//   - `string`           → text/plain;charset=UTF-8
//   - `Buffer`           → application/octet-stream
//   - `Uint8Array`       → application/octet-stream
//   - `ArrayBuffer`      → application/octet-stream
//   - plain object/array → application/json;charset=UTF-8 (auto-encoded)
//
// @example
//
//	const http = require("http");
//	const data = http.get("https://api.example.com/users").json();
//	const res  = http.post("https://api.example.com/users", { name: "alice" });
//	try {
//	    http.get("https://api.example.com/might-404");
//	} catch (e) {
//	    console.log(e.status);          // 404
//	    console.log(e.response.text()); // error body
//	}
func GetHTTPModule(vm *goja.Runtime, ctx context.Context, policy HTTPPolicy, transport *http.Transport) *goja.Object {
	env := &httpModule{
		vm:           vm,
		ctx:          ctx,
		maxBytes:     policy.MaxBodyBytes,
		allowedHosts: policy.AllowedHosts,
		client:       newTransformClient(policy, transport),
	}

	module := vm.NewObject()
	module.Set("get", func(call goja.FunctionCall) goja.Value {
		return env.verb("get", http.MethodGet, false, call)
	})
	module.Set("post", func(call goja.FunctionCall) goja.Value {
		return env.verb("post", http.MethodPost, true, call)
	})
	module.Set("put", func(call goja.FunctionCall) goja.Value {
		return env.verb("put", http.MethodPut, true, call)
	})
	module.Set("patch", func(call goja.FunctionCall) goja.Value {
		return env.verb("patch", http.MethodPatch, true, call)
	})
	module.Set("del", func(call goja.FunctionCall) goja.Value {
		return env.verb("del", http.MethodDelete, true, call)
	})
	module.Set("head", func(call goja.FunctionCall) goja.Value {
		return env.verb("head", http.MethodHead, false, call)
	})
	module.Set("Headers", NewHeadersConstructor(vm))
	return module
}

// @ts function http.get
// @doc Issue a GET request. Throws on non-2xx.
// @param url     - string | URL - The resource to request.
// @param headers? - Headers | Record<string,string> | [string,string][] - Request headers.
// @returns Response
//
// @ts function http.post
// @doc Issue a POST request. Throws on non-2xx.
// @param url     - string | URL - The resource to request.
// @param body?   - string | Buffer | Uint8Array | ArrayBuffer | object - Request body.
// @param headers? - Headers | Record<string,string> | [string,string][] - Request headers.
// @returns Response
//
// @ts function http.put
// @doc Issue a PUT request. Throws on non-2xx.
// @param url     - string | URL - The resource to request.
// @param body?   - string | Buffer | Uint8Array | ArrayBuffer | object - Request body.
// @param headers? - Headers | Record<string,string> | [string,string][] - Request headers.
// @returns Response
//
// @ts function http.patch
// @doc Issue a PATCH request. Throws on non-2xx.
// @param url     - string | URL - The resource to request.
// @param body?   - string | Buffer | Uint8Array | ArrayBuffer | object - Request body.
// @param headers? - Headers | Record<string,string> | [string,string][] - Request headers.
// @returns Response
//
// @ts function http.del
// @doc Issue a DELETE request. Throws on non-2xx. Named `del` because `delete` is a JS keyword.
// @param url     - string | URL - The resource to request.
// @param body?   - string | Buffer | Uint8Array | ArrayBuffer | object - Optional body.
// @param headers? - Headers | Record<string,string> | [string,string][] - Request headers.
// @returns Response
//
// @ts function http.head
// @doc Issue a HEAD request. Throws on non-2xx. Response body is always empty.
// @param url     - string | URL - The resource to request.
// @param headers? - Headers | Record<string,string> | [string,string][] - Request headers.
// @returns Response
func (e *httpModule) verb(name, method string, hasBody bool, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		panic(e.vm.NewTypeError("http.%s: url is required", name))
	}

	urlStr := call.Argument(0).String()

	parsed, parseErr := url.Parse(urlStr)
	if parseErr != nil {
		panic(e.vm.NewTypeError("http.%s: invalid URL: %q", name, parseErr))
	}
	if err := checkAllowedHost(parsed.Host, e.allowedHosts); err != nil {
		panic(e.vm.NewTypeError("http.%s: %v", name, err))
	}

	req := &Request{
		url:     urlStr,
		method:  method,
		headers: http.Header{},
	}

	var headerArg goja.Value
	if hasBody {
		bodyArg := call.Argument(1)
		headerArg = call.Argument(2)
		if !goja.IsUndefined(bodyArg) && !goja.IsNull(bodyArg) {
			body, ct, err := encodeBody(e.vm, bodyArg)
			if err != nil {
				panic(e.vm.NewTypeError("http.%s: body: %v", name, err))
			}
			req.body = body
			if ct != "" && req.headers.Get("Content-Type") == "" {
				req.headers.Set("Content-Type", ct)
			}
		}
	} else {
		headerArg = call.Argument(1)
	}

	if !goja.IsUndefined(headerArg) && !goja.IsNull(headerArg) {
		if err := populateHeaders(e.vm, req.headers, headerArg); err != nil {
			panic(e.vm.NewTypeError("http.%s: headers: %v", name, err))
		}
	}

	return e.execute(name, req)
}

// execute performs the HTTP round-trip and returns a JS Response.
// Throws a JS Error with .status and .response on non-2xx.
func (e *httpModule) execute(callerName string, req *Request) goja.Value {
	httpReq, err := req.toHTTPRequest(e.ctx)
	if err != nil {
		panic(e.vm.NewTypeError("http.%s: %v", callerName, err))
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		if ctxErr := e.ctx.Err(); ctxErr != nil {
			panic(e.vm.NewGoError(fmt.Errorf("http.%s: %w", callerName, ctxErr)))
		}
		panic(e.vm.NewGoError(fmt.Errorf("http.%s: %w", callerName, err)))
	}
	defer resp.Body.Close()

	body, err := readCappedBody(resp.Body, e.maxBytes)
	if err != nil {
		panic(e.vm.NewGoError(fmt.Errorf("http.%s: %w", callerName, err)))
	}

	finalURL := req.url
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	jsResp := newResponseObject(e.vm, &Response{
		status:     resp.StatusCode,
		statusText: http.StatusText(resp.StatusCode),
		headers:    &Headers{header: resp.Header, vm: e.vm},
		body:       body,
		url:        finalURL,
		redirected: finalURL != req.url,
		vm:         e.vm,
	})

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		e.throwHTTPError(callerName, resp.StatusCode, jsResp)
	}

	return jsResp
}

// throwHTTPError panics with a JS Error carrying .status and .response so
func (e *httpModule) throwHTTPError(callerName string, status int, response goja.Value) {
	msg := fmt.Sprintf("http.%s: HTTP %d %s", callerName, status, http.StatusText(status))
	errFn, ok := goja.AssertFunction(e.vm.GlobalObject().Get("Error"))
	if !ok {
		panic(e.vm.NewGoError(fmt.Errorf("%s", msg)))
	}
	errVal, err := errFn(goja.Undefined(), e.vm.ToValue(msg))
	if err != nil {
		panic(e.vm.NewGoError(fmt.Errorf("%s", msg)))
	}
	errObj := errVal.(*goja.Object)
	errObj.Set("status", status)
	errObj.Set("response", response)
	panic(errObj)
}

// readCappedBody reads up to max+1 bytes
func readCappedBody(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		return io.ReadAll(r)
	}
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("response body exceeds %d-byte limit", max)
	}
	return body, nil
}
