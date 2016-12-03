package cache

import (
	"net/http"
	"log"
)

type Transport struct {
	http.RoundTripper
	Cache
}

var _ http.RoundTripper = (*Transport)(nil)

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.RoundTripper
	if base == nil {
		base = http.DefaultTransport
	}

	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		t.Clear()
		return base.RoundTrip(req)
	}

	resp := t.Get(req)
	if resp != nil && valid(resp) {
		log.Print("cache hit")
		return resp, nil
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if cacheable(resp) {
		t.Set(req, resp)
	}

	return resp, nil
}

func valid(resp *http.Response) bool {
	return resp != nil
}

func cacheable(resp *http.Response) bool {
	return resp != nil
}
