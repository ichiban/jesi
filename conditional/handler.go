package conditional

import (
	"log"
	"net/http"
	"strings"

	"github.com/ichiban/jesi/common"
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
			log.Print(err)
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
