package cache

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHandler_ServeHTTP(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		handler *Handler
		req     *http.Request
		rep     *Representation
		cached  bool
	}{
		{ // fetch from backend and store
			handler: &Handler{
				Next: &testHandler{
					Resources: map[string]*Representation{
						"http://www.example.com/test": {
							StatusCode: http.StatusOK,
							HeaderMap: http.Header{
								"Cache-Control": []string{"public"},
							},
							Body: []byte(`{"foo":"bar"}`),
						},
					},
				},
				Store: &Store{},
			},
			req: &http.Request{
				Method: http.MethodGet,
				Header: http.Header{},
				URL:    url,
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"public"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			cached: true,
		},
		{ // fetch from backend and don't store
			handler: &Handler{
				Next: &testHandler{
					Resources: map[string]*Representation{
						"http://www.example.com/test": {
							StatusCode: http.StatusOK,
							HeaderMap: http.Header{
								"Cache-Control": []string{"private"},
							},
							Body: []byte(`{"foo":"bar"}`),
						},
					},
				},
				Store: &Store{},
			},
			req: &http.Request{
				Method: http.MethodGet,
				Header: http.Header{},
				URL:    url,
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"private"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			cached: false,
		},
		{ // fetch from store
			handler: &Handler{
				Next: &testHandler{},
				Store: &Store{
					Resources: map[ResourceKey]*Resource{
						{Host: "www.example.com", Path: "/test"}: {
							RepresentationKeys: map[RepresentationKey]struct{}{
								{Method: http.MethodGet, Key: ""}: {},
							},
						},
					},
					Representations: map[Key]*Representation{
						{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
							StatusCode: http.StatusOK,
							HeaderMap: http.Header{
								"Cache-Control": []string{"s-maxage=600"},
							},
							Body:         []byte(`{"foo":"bar"}`),
							RequestTime:  time.Now(),
							ResponseTime: time.Now(),
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				Header: http.Header{},
				URL:    url,
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap:  http.Header{},
				Body:       []byte(`{"foo":"bar"}`),
			},
			cached: true,
		},
	}

	for _, tc := range testCases {
		var rep Representation
		tc.handler.ServeHTTP(&rep, tc.req)

		if tc.rep.StatusCode != rep.StatusCode {
			t.Errorf("expected %d, got %d", tc.rep.StatusCode, rep.StatusCode)
		}

		for k := range tc.rep.HeaderMap {
			if len(tc.rep.HeaderMap[k]) != len(rep.HeaderMap[k]) {
				t.Errorf("expected %d, got %d", len(tc.rep.HeaderMap), len(rep.HeaderMap))
			}
			for i := range tc.rep.HeaderMap[k] {
				if tc.rep.HeaderMap[k][i] != rep.HeaderMap[k][i] {
					t.Errorf("for header %s, expected %#v, got %#v", k, tc.rep.HeaderMap[k][i], rep.HeaderMap[k][i])
				}
			}
		}

		if string(tc.rep.Body) != string(rep.Body) {
			t.Errorf("expected %#v, got %#v", string(tc.rep.Body), string(rep.Body))
		}

		cached := tc.handler.Get(tc.req)
		if tc.cached {
			if cached == nil {
				t.Error("expected to be cached, got nil")
			}
		} else {
			if cached != nil {
				t.Errorf("expected not to be cached, got %#v", cached)
			}
		}
	}
}

func TestCacheable(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		req    *http.Request
		rep    *Representation
		result bool
	}{
		{ // Non-GET requests are not cacheable.
			req: &http.Request{
				Method: http.MethodPost,
				URL:    url,
				Header: http.Header{},
				Body:   ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			rep: &Representation{
				StatusCode: http.StatusCreated,
				HeaderMap: http.Header{
					"Expires":  []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
					"Location": []string{"http://www.example.com/test"},
				},
			},
			result: false,
		},
		{ // Non-OK reponses are not cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			rep: &Representation{
				StatusCode: http.StatusNotFound,
				HeaderMap: http.Header{
					"Expires": []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
			},
			result: false,
		},
		{ // "no-store" requests are not cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{
					"Cache-Control": []string{"no-store"},
				},
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Expires": []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: false,
		},
		{ // "no-store" reponses are not cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"no-store"},
					"Expires":       []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: false,
		},
		{ // "private" reponses are not cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"private"},
					"Expires":       []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: false,
		},
		{ // Requests with Authorization header are not cacheable without an explicit cacheable response.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{
					"Authorization": []string{""},
				},
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Expires": []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: false,
		},
		{ // Requests with Authorization header are cacheable with an explicit cacheable response.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{
					"Authorization": []string{""},
				},
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"public"},
					"Expires":       []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: true,
		},
		{ // Responses with only Expires header are cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Expires": []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: true,
		},
		{ // Responses with "max-age" are cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"max-age=600"},
					"Expires":       []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: true,
		},
		{ // Responses with "s-maxage" are cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"s-maxage=600"},
					"Expires":       []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: true,
		},
		{ // Responses with "public" are cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			rep: &Representation{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"public"},
					"Expires":       []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: true,
		},
	}

	for i, tc := range testCases {
		result := Cacheable(tc.req, tc.rep)
		if tc.result != result {
			t.Errorf("(%d) expected %#v, got %#v", i, tc.result, result)
		}
	}
}

func TestHandler_State(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	testCases := []struct {
		originChangedAt time.Time
		req             *http.Request
		cached          *Representation

		state CachedState
		delta time.Duration
	}{
		{
			req: &http.Request{
				URL:    url,
				Header: http.Header{},
			},
			cached: nil,

			state: Miss,
			delta: time.Duration(0),
		},
		{
			req: &http.Request{
				URL: url,
				Header: http.Header{
					"Pragma": []string{"no-store"},
				},
			},
			cached: &Representation{
				HeaderMap:    http.Header{},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Revalidate,
			delta: time.Duration(0),
		},
		{
			req: &http.Request{
				URL: url,
				Header: http.Header{
					"Cache-Control": []string{"no-store"},
				},
			},
			cached: &Representation{
				HeaderMap:    http.Header{},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Revalidate,
			delta: time.Duration(0),
		},
		{
			req: &http.Request{
				URL:    url,
				Header: http.Header{},
			},
			cached: &Representation{
				HeaderMap: http.Header{
					"Cache-Control": []string{"no-store"},
				},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Revalidate,
			delta: time.Duration(0),
		},
		{
			originChangedAt: now,
			req: &http.Request{
				URL:    url,
				Header: http.Header{},
			},
			cached: &Representation{
				HeaderMap: http.Header{
					"Cache-Control": []string{"s-maxage=3"},
				},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Revalidate,
			delta: time.Duration(0),
		},
		{
			req: &http.Request{
				URL:    url,
				Header: http.Header{},
			},
			cached: &Representation{
				HeaderMap: http.Header{
					"Cache-Control": []string{"s-maxage=3"},
				},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Fresh,
			delta: -1 * time.Second,
		},
		{
			req: &http.Request{
				URL:    url,
				Header: http.Header{},
			},
			cached: &Representation{
				HeaderMap: http.Header{
					"Cache-Control": []string{"max-age=3"},
				},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Fresh,
			delta: -1 * time.Second,
		},
		{
			req: &http.Request{
				URL:    url,
				Header: http.Header{},
			},
			cached: &Representation{
				HeaderMap: http.Header{
					"Expires": []string{now.Add(3 * time.Second).Format(time.RFC1123)},
				},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Fresh,
			delta: -1 * time.Second,
		},
		{
			req: &http.Request{
				URL:    url,
				Header: http.Header{},
			},
			cached: &Representation{
				HeaderMap: http.Header{
					"Cache-Control": []string{"max-age=1, must-revalidate"},
				},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Revalidate,
			delta: time.Duration(0),
		},
		{
			req: &http.Request{
				URL:    url,
				Header: http.Header{},
			},
			cached: &Representation{
				HeaderMap: http.Header{
					"Cache-Control": []string{"max-age=2"},
				},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Stale,
			delta: time.Duration(0),
		},
		{
			req: &http.Request{
				URL:    url,
				Header: http.Header{},
			},
			cached: &Representation{
				HeaderMap:    http.Header{},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Revalidate,
			delta: time.Duration(0),
		},
	}

	for i, tc := range testCases {
		h := Handler{Store: &Store{OriginChangedAt: tc.originChangedAt}}
		s, d := h.State(tc.req, tc.cached)

		if tc.state != s {
			t.Errorf("(%d) expected %d, got %d", i, tc.state, s)
		}

		if tc.delta-d > 1*time.Millisecond {
			t.Errorf("(%d) expected %d, got %d", i, tc.delta, d)
		}
	}
}

type testHandler struct {
	Resources map[string]*Representation
}

var _ http.Handler = (*testHandler)(nil)

func (t *testHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		panic(req.Method)
	}

	rep, ok := t.Resources[req.URL.String()]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write(nil)
		return
	}

	rep.WriteTo(w)
}
