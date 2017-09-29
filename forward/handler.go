package forward

import (
	"io"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/ichiban/jesi/transaction"
)

// Handler forwards requests to backends.
type Handler struct {
	Transport http.RoundTripper
}

var _ http.Handler = (*Handler)(nil)

// ServeHTTP forwards requests to the backend.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := h.Transport.RoundTrip(r)
	if err != nil {
		log.WithFields(log.Fields{
			"id":    transaction.ID(r),
			"error": err,
		}).Error("failed to forward a request")

		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("failed to close a response body")
		}
	}()

	copyHeader(w.Header(), res.Header)

	w.WriteHeader(res.StatusCode)
	if _, err := io.Copy(w, res.Body); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("failed to copy a response body")
	}

	copyHeader(w.Header(), res.Trailer)
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
