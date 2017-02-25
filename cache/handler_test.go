package cache

import (
	"github.com/ichiban/jesi/common"
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
		resp    *common.ResponseBuffer
		cached  bool
	}{
		{ // fetch from backend and cache
			handler: &Handler{
				Next: &testHandler{
					Resources: map[string]*common.ResponseBuffer{
						"http://www.example.com/test": {
							StatusCode: http.StatusOK,
							HeaderMap: http.Header{
								"Cache-Control": []string{"public"},
							},
							Body: []byte(`{"foo":"bar"}`),
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				Header: http.Header{},
				URL:    url,
			},
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"public"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			cached: true,
		},
		{ // fetch from backend and don't cache
			handler: &Handler{
				Next: &testHandler{
					Resources: map[string]*common.ResponseBuffer{
						"http://www.example.com/test": {
							StatusCode: http.StatusOK,
							HeaderMap: http.Header{
								"Cache-Control": []string{"private"},
							},
							Body: []byte(`{"foo":"bar"}`),
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				Header: http.Header{},
				URL:    url,
			},
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"private"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			cached: false,
		},
		{ // fetch from cache
			handler: &Handler{
				Next: &testHandler{},
				Cache: Cache{
					URLVars: map[URLKey]*Variations{
						URLKey{Method: http.MethodGet, Host: "www.example.com", Path: "/test"}: {
							VarResponse: map[VarKey]*CachedResponse{
								"": {
									Header: http.Header{
										"Cache-Control": []string{"s-maxage=600"},
									},
									Body:         []byte(`{"foo":"bar"}`),
									RequestTime:  time.Now(),
									ResponseTime: time.Now(),
								},
							},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				Header: http.Header{},
				URL:    url,
			},
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap:  http.Header{},
				Body:       []byte(`{"foo":"bar"}`),
			},
			cached: true,
		},
	}

	for _, tc := range testCases {
		var resp common.ResponseBuffer
		tc.handler.ServeHTTP(&resp, tc.req)

		if tc.resp.StatusCode != resp.StatusCode {
			t.Errorf("expected %d, got %d", tc.resp.StatusCode, resp.StatusCode)
		}

		for k := range tc.resp.HeaderMap {
			if len(tc.resp.HeaderMap[k]) != len(resp.HeaderMap[k]) {
				t.Errorf("expected %d, got %d", len(tc.resp.HeaderMap), len(resp.HeaderMap))
			}
			for i := range tc.resp.HeaderMap[k] {
				if tc.resp.HeaderMap[k][i] != resp.HeaderMap[k][i] {
					t.Errorf("for header %s, expected %#v, got %#v", k, tc.resp.HeaderMap[k][i], resp.HeaderMap[k][i])
				}
			}
		}

		if string(tc.resp.Body) != string(resp.Body) {
			t.Errorf("expected %#v, got %#v", string(tc.resp.Body), string(resp.Body))
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
		resp   *common.ResponseBuffer
		result bool
	}{
		{ // Non-GET requests are not cacheable.
			req: &http.Request{
				Method: http.MethodPost,
				URL:    url,
				Header: http.Header{},
				Body:   ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusCreated,
				HeaderMap: http.Header{
					"Location": []string{"http://www.example.com/test"},
				},
			},
			result: false,
		},
		{ // Non-OK responses are not cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusNotFound,
				HeaderMap:  http.Header{},
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
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap:  http.Header{},
				Body:       []byte(`{"foo":"bar"}`),
			},
			result: false,
		},
		{ // "no-store" responses are not cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"no-store"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: false,
		},
		{ // "private" responses are not cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"private"},
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
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap:  http.Header{},
				Body:       []byte(`{"foo":"bar"}`),
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
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"public"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: true,
		},
		{ // Responses with Expires header are cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &common.ResponseBuffer{
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
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"max-age=600"},
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
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"s-maxage=600"},
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
			resp: &common.ResponseBuffer{
				StatusCode: http.StatusOK,
				HeaderMap: http.Header{
					"Cache-Control": []string{"public"},
				},
				Body: []byte(`{"foo":"bar"}`),
			},
			result: true,
		},
	}

	for i, tc := range testCases {
		result := Cacheable(tc.req, tc.resp)
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
		cached          *CachedResponse

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
					"Pragma": []string{"no-cache"},
				},
			},
			cached: &CachedResponse{
				Header:       http.Header{},
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
					"Cache-Control": []string{"no-cache"},
				},
			},
			cached: &CachedResponse{
				Header:       http.Header{},
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
			cached: &CachedResponse{
				Header: http.Header{
					"Cache-Control": []string{"no-cache"},
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
			cached: &CachedResponse{
				Header: http.Header{
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
			cached: &CachedResponse{
				Header: http.Header{
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
			cached: &CachedResponse{
				Header: http.Header{
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
			cached: &CachedResponse{
				Header: http.Header{
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
			cached: &CachedResponse{
				Header: http.Header{
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
			cached: &CachedResponse{
				Header: http.Header{
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
			cached: &CachedResponse{
				Header:       http.Header{},
				Body:         []byte{},
				RequestTime:  now.Add(-2 * time.Second),
				ResponseTime: now.Add(-1 * time.Second),
			},

			state: Revalidate,
			delta: time.Duration(0),
		},
	}

	for i, tc := range testCases {
		h := Handler{Cache: Cache{OriginChangedAt: tc.originChangedAt}}
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
	Resources map[string]*common.ResponseBuffer
}

var _ http.Handler = (*testHandler)(nil)

func (t *testHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		panic(req.Method)
	}

	resp, ok := t.Resources[req.URL.String()]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write(nil)
		return
	}

	resp.WriteTo(w)
}
