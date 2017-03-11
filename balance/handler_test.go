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

func TestState_String(t *testing.T) {
	testCases := []struct {
		state State
		str   string
	}{
		{state: StillHealthy, str: "StillHealthy"},
		{state: StillSick, str: "StillSick"},
		{state: BackHealthy, str: "BackHealthy"},
		{state: WentSick, str: "WentSick"},
		{state: -1, str: "Unknown"},
	}

	for _, tc := range testCases {
		if tc.str != tc.state.String() {
			t.Errorf("expected: %s, got: %s", tc.str, tc.state.String())
		}
	}
}

func TestBackend_Probe(t *testing.T) {
	testCases := []struct {
		backend  Backend
		statuses []int

		state State
	}{
		{
			backend: Backend{
				URL:   &url.URL{Path: "/foo"},
				State: StillHealthy,
			},
			statuses: []int{
				http.StatusOK,
			},

			state: StillHealthy,
		},
		{
			backend: Backend{
				URL:   &url.URL{Path: "/foo"},
				State: StillHealthy,
			},
			statuses: []int{
				http.StatusInternalServerError,
			},

			state: WentSick,
		},
		{
			backend: Backend{
				URL:   &url.URL{Path: "/foo"},
				State: StillSick,
			},
			statuses: []int{
				http.StatusOK,
			},

			state: BackHealthy,
		},
		{
			backend: Backend{
				URL:   &url.URL{Path: "/foo"},
				State: StillSick,
			},
			statuses: []int{
				http.StatusInternalServerError,
			},

			state: StillSick,
		},
		{
			backend: Backend{
				URL:   &url.URL{Path: "/foo"},
				State: BackHealthy,
			},
			statuses: []int{
				http.StatusOK,
			},

			state: StillHealthy,
		},
		{
			backend: Backend{
				URL:   &url.URL{Path: "/foo"},
				State: BackHealthy,
			},
			statuses: []int{
				http.StatusInternalServerError,
			},

			state: WentSick,
		},
		{
			backend: Backend{
				URL:   &url.URL{Path: "/foo"},
				State: WentSick,
			},
			statuses: []int{
				http.StatusOK,
			},

			state: BackHealthy,
		},
		{
			backend: Backend{
				URL:   &url.URL{Path: "/foo"},
				State: WentSick,
			},
			statuses: []int{
				http.StatusInternalServerError,
			},

			state: StillSick,
		},
	}

	for _, tc := range testCases {
		tc.backend.Transport = &testRoundTripper{
			statuses: tc.statuses,
		}
		tc.backend.Probe()
		if tc.state != tc.backend.State {
			t.Errorf("expected: %s, got: %s", tc.state, tc.backend.State)
		}

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
