package common

import (
	"io"
	"net/http"
)

// ResponseBuffer is a buffered response.
type ResponseBuffer struct {
	StatusCode int
	HeaderMap  http.Header
	Body       []byte
}

var _ http.ResponseWriter = (*ResponseBuffer)(nil)
var _ io.WriterTo = (*ResponseBuffer)(nil)

// Header returns HTTP header.
func (r *ResponseBuffer) Header() http.Header {
	if r.HeaderMap == nil {
		r.HeaderMap = http.Header{}
	}
	return r.HeaderMap
}

// Write writes data into the buffer.
func (r *ResponseBuffer) Write(b []byte) (int, error) {
	r.Body = append(r.Body, b...)
	return len(b), nil
}

// WriteHeader stores status code.
func (r *ResponseBuffer) WriteHeader(code int) {
	r.StatusCode = code
}

// WriteTo writes out the contents of the buffer to io.Writer.
// it also writes status code and header if w is an http.ResponseWriter.
func (r *ResponseBuffer) WriteTo(w io.Writer) (int64, error) {
	rw, ok := w.(http.ResponseWriter)
	if !ok {
		n, err := w.Write(r.Body)
		return int64(n), err
	}

	h := rw.Header()
	for k, v := range r.HeaderMap {
		h[k] = v
	}

	rw.WriteHeader(r.StatusCode)
	n, err := rw.Write(r.Body)
	return int64(n), err
}

// Successful returns true if status code is 2xx or 3xx.
func (r *ResponseBuffer) Successful() bool {
	return 200 <= r.StatusCode && r.StatusCode < 400
}
