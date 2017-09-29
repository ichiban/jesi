package balance

import (
	"container/list"
	"net/http"
	"net/url"
	"testing"
	"time"
	"io/ioutil"
	"bytes"
)

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
		s int
		q string
	}{
		{
			s: http.StatusOK,
			q: "healthy",
		},
		{
			s: http.StatusInternalServerError,
			q: "sick",
		},
	}

	for _, tc := range testCases {
		var p BackendPool
		p.RoundTripper = &testRoundTripper{statuses: []int{tc.s}}
		p.Set("http://example.com/foo")

		var queue string
		if p.Healthy.Len() > 0 {
			queue = "healthy"
		}
		if p.Sick.Len() > 0 {
			queue = "sick"
		}

		if tc.q != queue {
			t.Errorf("expected %s, got %s", tc.q, queue)
		}
	}

}

func TestBackendPool_Add(t *testing.T) {
	testCases := []struct {
		b    Backend
		sick bool
	}{
		{
			b: Backend{
				URL: &url.URL{Path: "/foo"},
				Client: http.Client{
					Transport: &testRoundTripper{statuses: []int{http.StatusOK}},
				},
			},
			sick: false,
		},
		{
			b: Backend{
				URL: &url.URL{Path: "/foo"},
				Client: http.Client{
					Transport: &testRoundTripper{statuses: []int{http.StatusInternalServerError}},
				},
			},
			sick: true,
		},
	}

	for _, tc := range testCases {
		var p BackendPool
		p.Add(&tc.b)

		var l *list.List
		if tc.sick {
			l = &p.Sick
		} else {
			l = &p.Healthy
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
			b.Client = http.Client{Transport: &testRoundTripper{statuses: []int{http.StatusOK}}}
			p.Add(b)
		}

		for _, b := range tc.beforeSick {
			b.Client = http.Client{Transport: &testRoundTripper{statuses: []int{http.StatusInternalServerError}}}
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
				if h.URL.String() != healthy[j].URL.String() {
					t.Errorf("(%d, %d) expected: %s, got: %s", i, j, h.URL, healthy[j].URL)
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
				if s.URL.String() != sick[j].URL.String() {
					t.Errorf("(%d, %d) expected: %s, got: %s", i, j, s.URL, sick[j].URL)
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
