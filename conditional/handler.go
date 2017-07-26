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

	etag := strings.Trim(r.Header.Get(ifNoneMatchField), " ")
	if etag == "" {
		h.Next.ServeHTTP(w, r)
		return
	}

	var resp common.ResponseBuffer
	h.Next.ServeHTTP(&resp, r)

	if etag != strings.Trim(resp.HeaderMap.Get(etagField), " ") {
		if _, err := resp.WriteTo(w); err != nil {
			log.WithFields(log.Fields{
				"request": request.ID(r),
				"error":   err,
			}).Error("Couldn't write a response")
		}
		return
	}

	resp.StatusCode = http.StatusNotModified
	delete(resp.HeaderMap, contentTypeField)
	delete(resp.HeaderMap, contentLengthField)
	resp.Body = nil
	if _, err := resp.WriteTo(w); err != nil {
		log.Print(err)
	}
}
