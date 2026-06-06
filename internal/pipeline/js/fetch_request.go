package js

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/dop251/goja"
)

// Request is the internal representation of an outbound HTTP request.
type Request struct {
	url     string
	method  string
	headers http.Header
	body    []byte
}

// toHTTPRequest converts the internal Request into a stdlib *http.Request,
func (r *Request) toHTTPRequest(ctx context.Context) (*http.Request, error) {
	var bodyReader io.Reader
	if len(r.body) > 0 {
		bodyReader = bytes.NewReader(r.body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, r.method, r.url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building http request: %w", err)
	}
	if r.headers != nil {
		httpReq.Header = r.headers.Clone()
	}
	return httpReq, nil
}

var reflectTypeString = reflect.TypeOf("")

// encodeBody converts a JS body value into wire bytes and infers the
// Content-Type when the caller has not set one explicitly.
//
// Detection order:
//  1. nil / undefined / null            : no body
//  2. string                            : text/plain;charset=UTF-8
//  3. goja.ArrayBuffer                  : application/octet-stream
//  4. Buffer / Uint8Array / typed array : application/octet-stream
//  5. anything else                     : application/json;charset=UTF-8
func encodeBody(vm *goja.Runtime, val goja.Value) (body []byte, contentType string, err error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, "", nil
	}

	// Strings: fast path, most common body type.
	if val.ExportType() == reflectTypeString {
		return []byte(val.String()), "text/plain;charset=UTF-8", nil
	}

	// Raw goja.ArrayBuffer
	if ab, ok := val.Export().(goja.ArrayBuffer); ok {
		src := ab.Bytes()
		out := make([]byte, len(src))
		copy(out, src)
		return out, "application/octet-stream", nil
	}

	// Buffer / Uint8Array / any ArrayBuffer-backed typed array.
	if _, ok := val.(*goja.Object); ok {
		var buf []byte
		if vm.ExportTo(val, &buf) == nil {
			out := make([]byte, len(buf))
			copy(out, buf)
			return out, "application/octet-stream", nil
		}
	}

	// Fallback: JSON-encode everything else
	encoded, jerr := json.Marshal(val.Export())
	if jerr != nil {
		return nil, "", fmt.Errorf("encoding body: %w", jerr)
	}
	return encoded, "application/json;charset=UTF-8", nil
}
