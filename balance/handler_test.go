package balance

import (
	"bytes"
	"github.com/ichiban/jesi/common"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"
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
				{URL: &url.URL{Scheme: "https", Host: "p.example.com"}},
				{URL: &url.URL{Scheme: "https", Host: "c.example.com"}},
			},
			reqURL:  url.URL{Path: "/foo"},
			numReqs: 6,

			directed: []url.URL{
				{Scheme: "https", Host: "a.example.com", Path: "/foo"},
				{Scheme: "https", Host: "p.example.com", Path: "/foo"},
				{Scheme: "https", Host: "c.example.com", Path: "/foo"},
				{Scheme: "https", Host: "a.example.com", Path: "/foo"},
				{Scheme: "https", Host: "p.example.com", Path: "/foo"},
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
			p.Add(b)
		}

		h := Handler{BackendPool: &p}

		rt := testRoundTripper{}

		h.ReverseProxy.Transport = &rt

		for i := 0; i < tc.numReqs; i++ {
			var resp common.ResponseBuffer
			h.ServeHTTP(&resp, &http.Request{
				URL:    &tc.reqURL,
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

func TestBackend_Probe(t *testing.T) {
	testCases := []struct {
		backend  Backend
		statuses []int

		sick bool
	}{
		{
			backend: Backend{
				URL:  &url.URL{Path: "/foo"},
				Sick: false,
			},
			statuses: []int{
				http.StatusOK,
			},

			sick: false,
		},
		{
			backend: Backend{
				URL:  &url.URL{Path: "/foo"},
				Sick: false,
			},
			statuses: []int{
				http.StatusInternalServerError,
			},

			sick: true,
		},
		{
			backend: Backend{
				URL:  &url.URL{Path: "/foo"},
				Sick: true,
			},
			statuses: []int{
				http.StatusInternalServerError,
			},

			sick: true,
		},
		{
			backend: Backend{
				URL:  &url.URL{Path: "/foo"},
				Sick: true,
			},
			statuses: []int{
				http.StatusOK,
			},

			sick: false,
		},
	}

	for _, tc := range testCases {
		tc.backend.Transport = &testRoundTripper{
			statuses: tc.statuses,
		}
		tc.backend.Probe()
		if tc.sick != tc.backend.Sick {
			t.Errorf("expected: %t, got: %t", tc.sick, tc.backend.Sick)
		}

	}
}

func TestBackend_Run(t *testing.T) {
	testCases := []struct {
		backend  Backend
		statuses []int
	}{
		{
			backend: Backend{
				URL:  &url.URL{Path: "/foo"},
				Sick: false,
			},
			statuses: []int{
				http.StatusInternalServerError,
			},
		},
		{
			backend: Backend{
				URL:  &url.URL{Path: "/foo"},
				Sick: false,
			},
			statuses: []int{
				http.StatusOK,
				http.StatusInternalServerError,
			},
		},
		{
			backend: Backend{
				URL:  &url.URL{Path: "/foo"},
				Sick: true,
			},
			statuses: []int{
				http.StatusOK,
			},
		},
		{
			backend: Backend{
				URL:  &url.URL{Path: "/foo"},
				Sick: true,
			},
			statuses: []int{
				http.StatusInternalServerError,
				http.StatusOK,
			},
		},
	}

	for _, tc := range testCases {
		transport := testRoundTripper{
			statuses: tc.statuses,
		}
		tc.backend.Interval = time.Nanosecond
		tc.backend.Transport = &transport

		ch := make(chan *Backend)
		quit := make(chan struct{})

		go tc.backend.Run(ch, quit)

		<-ch

		if len(tc.statuses) != len(transport.urls) {
			t.Errorf("expedted: %d, got: %d", len(tc.statuses), len(transport.urls))
		}

		quit <- struct{}{}
	}
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
