package balance

import (
	"bytes"
	"container/list"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/ichiban/jesi/common"
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

		timer := make(chan time.Time)
		tc.backend.Timer = timer
		tc.backend.Transport = &transport

		old := tc.backend.Sick

		ch := make(chan *Backend)
		q := make(chan struct{})

		go tc.backend.Run(ch, q)

		for range tc.statuses {
			timer <- time.Now()
		}
		b := <-ch
		close(q)

		if &tc.backend != b {
			t.Errorf("expected: %#v, got: %#v", &tc.backend, b)
		}

		if old == b.Sick {
			t.Errorf("expected: %t, got: %t", !old, b.Sick)
		}
	}
}

func TestBackendPool_Set(t *testing.T) {
	testCases := []struct {
		p BackendPool
		b string
	}{
		{
			p: BackendPool{},
			b: "http://example.com/foo",
		},
	}

	for _, tc := range testCases {
		tc.p.Set(tc.b)

		var found bool
		for e := tc.p.Healthy.Front(); e != nil; e.Next() {
			b := e.Value.(*Backend)
			if b.URL.String() == tc.b {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected found, got not found")
		}
	}

}

func TestBackendPool_Add(t *testing.T) {
	testCases := []struct {
		p    BackendPool
		b    Backend
		sick bool
	}{
		{
			p: BackendPool{},
			b: Backend{
				Client: http.Client{
					Transport: &testRoundTripper{},
				},
				Sick: false,
			},
			sick: false,
		},
		{
			p: BackendPool{},
			b: Backend{
				Client: http.Client{
					Transport: &testRoundTripper{},
				},
				Sick: true,
			},
			sick: true,
		},
	}

	for _, tc := range testCases {
		tc.p.Add(&tc.b)

		var l *list.List
		if tc.sick {
			l = &tc.p.Sick
		} else {
			l = &tc.p.Healthy
		}

		var found bool
		for e := l.Front(); e != nil; e.Next() {
			b := e.Value.(*Backend)
			if b == &tc.b {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected found, got not found")
		}
	}

}

func TestBackendPool_Next(t *testing.T) {
	a := &Backend{}
	b := &Backend{}
	c := &Backend{}

	testCases := []struct {
		before []*Backend
		next   *Backend
		after  []*Backend
	}{
		{
			before: []*Backend{},
			next:   nil,
			after:  []*Backend{},
		},
		{
			before: []*Backend{a},
			next:   a,
			after:  []*Backend{a},
		},
		{
			before: []*Backend{a, b, c},
			next:   a,
			after:  []*Backend{b, c, a},
		},
	}

	for i, tc := range testCases {
		var p BackendPool

		for _, b := range tc.before {
			b.Element = p.Healthy.PushBack(b)
		}

		n := p.Next()

		if tc.next != n {
			t.Errorf("(%d) expected: %#v, got: %#v", i, tc.next, n)
		}

		var result []*Backend
		for e := p.Healthy.Front(); e != nil; e = e.Next() {
			result = append(result, e.Value.(*Backend))
		}

		if len(tc.after) != len(result) {
			t.Errorf("(%d) expected: %d, got: %d", i, len(tc.after), len(result))
		}

		for j, a := range tc.after {
			if a != tc.after[j] {
				t.Errorf("(%d, %d) expected: %#v, got: %#v", i, j, a, tc.after[j])
			}
		}
	}
}

func TestBackendPool_Run(t *testing.T) {
	a := &Backend{
		URL: &url.URL{Path: "/foo"},
		Client: http.Client{
			Transport: &testRoundTripper{},
		},
		Interval: time.Hour,
	}
	b := &Backend{
		URL: &url.URL{Path: "/foo"},
		Client: http.Client{
			Transport: &testRoundTripper{},
		},
		Interval: time.Hour,
	}

	testCases := []struct {
		beforeHealthy []*Backend
		beforeSick    []*Backend

		change []*Backend

		afterHealthy []*Backend
		afterSick    []*Backend
	}{
		{
			beforeHealthy: []*Backend{},
			beforeSick:    []*Backend{},

			change: []*Backend{},

			afterHealthy: []*Backend{},
			afterSick:    []*Backend{},
		},
		{
			beforeHealthy: []*Backend{a},
			beforeSick:    []*Backend{b},

			change: []*Backend{a, b},

			afterHealthy: []*Backend{b},
			afterSick:    []*Backend{a},
		},
	}

	for i, tc := range testCases {
		var p BackendPool

		for _, b := range tc.beforeHealthy {
			b.Sick = false
			p.Add(b)
		}

		for _, b := range tc.beforeSick {
			b.Sick = true
			p.Add(b)
		}

		ch := make(chan struct{})

		go p.Run(ch)

		<-ch

		for _, b := range tc.change {
			b.Sick = !b.Sick
			p.Changed <- b
			<-ch
		}

		close(ch)

		{
			var healthy []*Backend
			for e := p.Healthy.Front(); e != nil; e = e.Next() {
				b := e.Value.(*Backend)
				healthy = append(healthy, b)
			}

			if len(tc.afterHealthy) != len(healthy) {
				t.Errorf("(%d) expected: %d, got: %d", i, len(tc.afterHealthy), len(healthy))
			}

			for j, h := range tc.afterHealthy {
				if h != healthy[j] {
					t.Errorf("(%d, %d) expected: %#v, got: %#v", i, j, h, healthy[j])
				}
			}
		}
		{
			var sick []*Backend
			for e := p.Sick.Front(); e != nil; e = e.Next() {
				b := e.Value.(*Backend)
				sick = append(sick, b)
			}

			if len(tc.afterSick) != len(sick) {
				t.Errorf("(%d) expected: %d, got: %d", i, len(tc.afterSick), len(sick))
			}

			for j, s := range tc.afterSick {
				if s != sick[j] {
					t.Errorf("(%d, %d) expected: %#v, got: %#v", i, j, s, sick[j])
				}
			}
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
