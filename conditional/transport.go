package conditional

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	ifNoneMatchField   = "If-None-Match"
	etagField          = "ETag"
	contentTypeField   = "Content-Type"
	contentLengthField = "Content-Length"
)

// Transport is a Conditional GET (ETag only) round tripper.
type Transport struct {
	http.RoundTripper
}

var _ http.RoundTripper = (*Transport)(nil)

// RoundTrip returns NotModified if ETag matches.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.RoundTripper
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return resp, nil
	}

	etag := strings.Trim(req.Header.Get(ifNoneMatchField), " ")
	if etag == "" {
		return resp, nil
	}

	if etag != strings.Trim(resp.Header.Get(etagField), " ") {
		return resp, nil
	}

	if err := resp.Body.Close(); err != nil {
		return resp, err
	}

	resp.StatusCode = http.StatusNotModified
	delete(resp.Header, contentTypeField)
	delete(resp.Header, contentLengthField)
	resp.Body = ioutil.NopCloser(bytes.NewReader(nil))

	return resp, nil
}
