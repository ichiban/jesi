package transaction

import (
	"net/http"

	log "github.com/sirupsen/logrus"
)

// Handler is a Conditional GET (ETag only) handler.
type Handler struct {
	Type string
	Next http.Handler
}

var _ http.Handler = (*Handler)(nil)

// ServeHTTP returns NotModified if ETag matches.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parent := ID(r)
	r = WithID(r)
	id := ID(r)
	log.WithFields(log.Fields{
		"id":             id,
		"type":           h.Type,
		"parent":         parent,
		"method":         r.Method,
		"host":           r.Host,
		"url":            r.URL,
		"content_length": r.ContentLength,
		"remote_addr":    r.RemoteAddr,
		"header":         r.Header,
	}).Info("Started a transaction")

	rw := &responseWriter{
		ResponseWriter: w,
	}
	h.Next.ServeHTTP(rw, r)

	log.WithFields(log.Fields{
		"id":             id,
		"status":         rw.status,
		"content_length": rw.contentLength,
		"header":         rw.Header(),
	}).Info("Finished a transaction")
}

type responseWriter struct {
	http.ResponseWriter

	status        int
	contentLength int64
}

func (w *responseWriter) WriteHeader(s int) {
	w.status = s
	w.ResponseWriter.WriteHeader(s)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.contentLength += int64(n)
	return n, err
}
