package balance

import (
	"bytes"
	"github.com/ichiban/jesi/common"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
)

func TestHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		backends Backends
		numReqs  int

		directed []url.URL
	}{
		{
			backends: Backends{
				All: []Backend{
					{URL: &url.URL{Scheme: "https", Host: "a.example.com"}},
					{URL: &url.URL{Scheme: "https", Host: "b.example.com"}},
					{URL: &url.URL{Scheme: "https", Host: "c.example.com"}},
				},
			},
			numReqs: 6,

			directed: []url.URL{
				{Scheme: "https", Host: "a.example.com"},
				{Scheme: "https", Host: "b.example.com"},
				{Scheme: "https", Host: "c.example.com"},
				{Scheme: "https", Host: "a.example.com"},
				{Scheme: "https", Host: "b.example.com"},
				{Scheme: "https", Host: "c.example.com"},
			},
		},
	}

	for i, tc := range testCases {
		h := Handler{
			Backends: &tc.backends,
		}

		rt := testRoundTripper{}

		h.ReverseProxy.Transport = &rt

		for i := 0; i < tc.numReqs; i++ {
			var resp common.ResponseBuffer
			h.ServeHTTP(&resp, &http.Request{
				URL:    &url.URL{},
				Header: http.Header{},
			})
		}

		if len(tc.directed) != len(rt.urls) {
			t.Errorf("(%d) expect: %d, got: %d", i, len(tc.directed), len(rt.urls))
			continue
		}

		for j, u := range tc.directed {
			if u.String() != rt.urls[j].String() {
				t.Errorf("(%d) expect: %s, got: %s", i, u, rt.urls[j])
			}
		}
	}
}

type testRoundTripper struct {
	urls []url.URL
}

var _ http.RoundTripper = (*testRoundTripper)(nil)

func (t *testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.urls = append(t.urls, *req.URL)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       ioutil.NopCloser(bytes.NewBuffer(nil)),
	}, nil
}
