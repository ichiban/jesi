package cache

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/satori/go.uuid"
)

// Representation is a buffered response.
type Representation struct {
	sync.RWMutex
	ResourceKey
	RepresentationKey
	ID           uuid.UUID
	StatusCode   int
	HeaderMap    http.Header
	Body         []byte
	RequestTime  time.Time
	ResponseTime time.Time
	LastUsedTime time.Time
}

var _ http.ResponseWriter = (*Representation)(nil)
var _ io.WriterTo = (*Representation)(nil)

// NewRepresentation creates a new representation from a handler and request.
func NewRepresentation(h http.Handler, r *http.Request) *Representation {
	id, _ := uuid.NewV4()
	rep := Representation{
		ID:        id,
		HeaderMap: http.Header{},
	}
	rep.RequestTime = time.Now()
	h.ServeHTTP(&rep, r)
	rep.ResponseTime = time.Now()
	return &rep
}

// Header returns HTTP header.
func (r *Representation) Header() http.Header {
	if r.HeaderMap == nil {
		r.HeaderMap = http.Header{}
	}
	return r.HeaderMap
}

// Write writes data into the buffer.
func (r *Representation) Write(b []byte) (int, error) {
	r.Body = append(r.Body, b...)
	return len(b), nil
}

// WriteHeader stores status code.
func (r *Representation) WriteHeader(code int) {
	r.StatusCode = code
}

// WriteTo writes out the contents of the buffer to io.Writer.
// it also writes status code and header if w is an http.responseWriter.
func (r *Representation) WriteTo(w io.Writer) (int64, error) {
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
func (r *Representation) Successful() bool {
	return 200 <= r.StatusCode && r.StatusCode < 400
}

func (r *Representation) clone() *Representation {
	var c Representation
	c.StatusCode = r.StatusCode
	c.HeaderMap = make(http.Header, len(r.HeaderMap))
	for k, vs := range r.HeaderMap {
		cvs := make([]string, len(vs))
		copy(cvs, vs)
		c.HeaderMap[k] = cvs
	}
	c.RequestTime = r.RequestTime
	c.ResponseTime = r.ResponseTime
	c.Body = r.Body
	return &c
}
