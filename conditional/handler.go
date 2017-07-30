package conditional

import (
	"net/http"
	"strings"

	"github.com/ichiban/jesi/common"
	"github.com/ichiban/jesi/request"
	log "github.com/sirupsen/logrus"
)

const (
	ifNoneMatchField   = "If-None-Match"
	etagField          = "ETag"
	contentTypeField   = "Content-Type"
	contentLengthField = "Content-Length"
)

// Handler is a Conditional GET (ETag only) handler.
type Handler struct {
	Next http.Handler
}

var _ http.Handler = (*Handler)(nil)

// ServeHTTP returns NotModified if ETag matches.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.WithFields(log.Fields{
		"request": request.ID(r),
	}).Debug("Will serve not modified if so")

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.Next.ServeHTTP(w, r)
		return
	}

	etag := strings.TrimSpace(r.Header.Get(ifNoneMatchField))
	if etag == "" {
		h.Next.ServeHTTP(w, r)
		return
	}

	var resp common.ResponseBuffer
	h.Next.ServeHTTP(&resp, r)
	defer func() {
		if _, err := resp.WriteTo(w); err != nil {
			log.WithFields(log.Fields{
				"request": request.ID(r),
				"error":   err,
			}).Error("Couldn't write a response")
		}
	}()

	if etag != strings.TrimSpace(resp.HeaderMap.Get(etagField)) {
		return
	}

	resp.StatusCode = http.StatusNotModified
	delete(resp.HeaderMap, contentTypeField)
	delete(resp.HeaderMap, contentLengthField)
	resp.Body = nil
}
