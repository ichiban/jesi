package balance

import (
	"net/http"
	"net/http/httputil"
	"path"

	"github.com/ichiban/jesi/transaction"
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
			"id": transaction.ID(r),
		}).Warn("Couldn't find a backend in the pool")

		return
	}

	log.WithFields(log.Fields{
		"id":      transaction.ID(r),
		"backend": b,
	}).Debug("Picked up a backend from the pool")

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
		"id":  transaction.ID(r),
		"url": r.URL,
	}).Debug("Directed a request to a backend")
}
