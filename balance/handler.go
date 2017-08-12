package balance

import (
	"net/http"
	"net/http/httputil"
	"path"

	"github.com/ichiban/jesi/request"
	log "github.com/sirupsen/logrus"
)

// Handler is a reverse proxy with multiple backends.
type Handler struct {
	httputil.ReverseProxy
	*BackendPool
}

var _ http.Handler = (*Handler)(nil)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Director == nil {
		h.Director = h.direct
	}

	h.ReverseProxy.ServeHTTP(w, r)
}

func (h *Handler) direct(r *http.Request) {
	b := h.BackendPool.Next()

	if b == nil {
		log.WithFields(log.Fields{
			"request": request.ID(r),
		}).Warn("Couldn't find a backend in the pool")

		return
	}

	log.WithFields(log.Fields{
		"request": request.ID(r),
		"backend": b,
	}).Info("Picked up a backend from the pool")

	r.URL.Scheme = b.URL.Scheme
	r.URL.Host = b.URL.Host
	r.URL.Path = path.Join(b.URL.Path, r.URL.Path)
	if b.URL.RawQuery == "" || r.URL.RawQuery == "" {
		r.URL.RawQuery = b.URL.RawQuery + r.URL.RawQuery
	} else {
		r.URL.RawQuery = b.URL.RawQuery + "&" + r.URL.RawQuery
	}
	if _, ok := r.Header["User-Agent"]; !ok {
		// explicitly disable User-Agent so it's not set to default value
		r.Header.Set("User-Agent", "")
	}

	log.WithFields(log.Fields{
		"request": request.ID(r),
		"url":     r.URL,
	}).Info("Directed a request to a backend")
}
