package cache

import (
	"log"
	"net/http"
	"regexp"
	"strings"
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
		log.Printf("from cache: %s", req.URL)
		return resp, nil
	}

	log.Printf("from backend: %s", req.URL)

	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if cacheable(req, resp) {
		t.Set(req, resp)
	}

	return resp, nil
}

func valid(resp *http.Response) bool {
	// TODO: implement
	return resp != nil
}

// check if the req/resp pair is cacheable based on https://tools.ietf.org/html/rfc7234#section-3
func cacheable(req *http.Request, resp *http.Response) bool {
	if req.Method != http.MethodGet {
		return false
	}

	if resp.StatusCode != http.StatusOK {
		return false
	}

	if contains(req.Header, "Cache-Control", regexp.MustCompile(`\Ano-cache\z`)) {
		return false
	}

	if contains(resp.Header, "Cache-Control", regexp.MustCompile(`\A(?:no-cache|private)\z`)) {
		return false
	}

	if _, ok := req.Header["Authorization"]; ok {
		if !contains(resp.Header, "Cache-Control", regexp.MustCompile(`\A(?:must-revalidate|public|s-maxage=\d+)\z`)) {
			return false
		}
	}

	if _, ok := resp.Header["Expires"]; ok {
		return true
	}

	if contains(resp.Header, "Cache-Control", regexp.MustCompile(`\A(?:maxage=\d+|s-maxage=\d+|public)\z`)) {
		return true
	}

	return false
}

func contains(h http.Header, key string, value *regexp.Regexp) bool {
	vs, ok := h[key]
	if !ok {
		return false
	}

	for _, v := range vs {
		vs := strings.Split(v, ",")

		for _, v := range vs {
			v := strings.Trim(v, " ")
			if value.MatchString(v) {
				return true
			}
		}
	}

	return false
}
