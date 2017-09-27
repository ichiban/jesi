package balance

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"

	"github.com/ichiban/jesi/cache"
)

func TestHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		backends []*Backend
		reqURL   url.URL
		numReqs  int

		directed []url.URL
	}{
		{ // if there's no backend available, it doesn't anything.
			backends: []*Backend{},
			reqURL:   url.URL{Path: "/foo"},
			numReqs:  1,

			directed: []url.URL{
				{Path: "/foo"},
			},
		},
		{ // if there're multiple backends available, it spreads the workload across them.
			backends: []*Backend{
				{URL: &url.URL{Scheme: "https", Host: "a.example.com"}},
				{URL: &url.URL{Scheme: "https", Host: "b.example.com"}},
				{URL: &url.URL{Scheme: "https", Host: "c.example.com"}},
			},
			reqURL:  url.URL{Path: "/foo"},
			numReqs: 6,

			directed: []url.URL{
				{Scheme: "https", Host: "a.example.com", Path: "/foo"},
				{Scheme: "https", Host: "b.example.com", Path: "/foo"},
				{Scheme: "https", Host: "c.example.com", Path: "/foo"},
				{Scheme: "https", Host: "a.example.com", Path: "/foo"},
				{Scheme: "https", Host: "b.example.com", Path: "/foo"},
				{Scheme: "https", Host: "c.example.com", Path: "/foo"},
			},
		},
		{ // if there're query strings in the backend URL or the request URL, it combines them.
			backends: []*Backend{
				{URL: &url.URL{Scheme: "https", Host: "a.example.com", RawQuery: "a=0&b=1"}},
			},
			reqURL:  url.URL{Path: "/foo", RawQuery: "c=2&d=3"},
			numReqs: 1,

			directed: []url.URL{
				{Scheme: "https", Host: "a.example.com", Path: "/foo", RawQuery: "a=0&b=1&c=2&d=3"},
			},
		},
	}

	for i, tc := range testCases {
		var p BackendPool
		for _, b := range tc.backends {
			b.Client = http.Client{Transport: &testRoundTripper{statuses: []int{http.StatusOK}}}
			p.Add(b)
		}

		h := Handler{BackendPool: &p}

		th := &testHandler{}

		h.Next = th

		for i := 0; i < tc.numReqs; i++ {
			var rep cache.Representation
			h.ServeHTTP(&rep, &http.Request{
				URL:    &tc.reqURL,
				Header: http.Header{},
			})
		}

		if len(tc.directed) != len(th.urls) {
			t.Errorf("(%d) expect: %d, got: %d", i, len(tc.directed), len(th.urls))
			continue
		}

		for j, u := range tc.directed {
			if u.String() != th.urls[j].String() {
				t.Errorf("(%d) expect: %s, got: %s", i, u, th.urls[j])
			}
		}
	}
}

type testHandler struct {
	statuses []int
	urls     []url.URL
}

var _ http.Handler = (*testHandler)(nil)

func (t *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.urls = append(t.urls, *r.URL)

	var status int
	if len(t.statuses) == 0 {
		status = http.StatusOK
	} else {
		status = t.statuses[0]
		t.statuses = t.statuses[1:]
	}

	w.WriteHeader(status)
}

type testRoundTripper struct {
	statuses []int
	urls     []url.URL
}

var _ http.RoundTripper = (*testRoundTripper)(nil)

func (t *testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.urls = append(t.urls, *req.URL)

	var status int
	if len(t.statuses) == 0 {
		status = http.StatusOK
	} else {
		status = t.statuses[0]
		t.statuses = t.statuses[1:]
	}

	return &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       ioutil.NopCloser(bytes.NewBuffer(nil)),
	}, nil
}
