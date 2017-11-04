package embed

import (
	"net/http"
	"testing"

	"github.com/ichiban/jesi/cache"
	"net/url"
)

func TestHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		req       *http.Request
		resources map[string]*testResource
		resp      *cache.Representation
	}{
		{ // without 'with' query parameter, it simply returns JSON.
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/test",
				},
			},
			resources: map[string]*testResource{
				"/test": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{}`,
				},
			},
			resp: &cache.Representation{
				Body: []byte(`{}`),
			},
		},
		{ // with 'with' query parameter, it embeds resources specified by edges.
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path:     "/a",
					RawQuery: "with=foo.bar.baz",
				},
			},
			resources: map[string]*testResource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
				"/b": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"bar":{"href":"/c"},"self":{"href":"/b"}}}`,
				},
				"/c": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"next":{"href":"/a"},"self":{"href":"/c"}}}`,
				},
			},
			resp: &cache.Representation{
				Body: []byte(`{"_embedded":{"foo":{"_embedded":{"bar":{"_embedded":{},"_links":{"next":{"href":"/a"},"self":{"href":"/c"}}}},"_links":{"bar":{"href":"/c"},"self":{"href":"/b"}}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`),
			},
		},
		{ // multiple 'with' query parameters are also fine.
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path:     "/a",
					RawQuery: "with=foo.bar.baz&with=foo.qux.quux",
				},
			},
			resources: map[string]*testResource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
				"/b": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"bar":{"href":"/c"},"qux":{"href":"/d"},"self":{"href":"/b"}}}`,
				},
				"/c": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"baz":{"href":"/a"},"self":{"href":"/c"}}}`,
				},
				"/d": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"quux":{"href":"/e"},"self":{"href":"/d"}}}`,
				},
				"/e": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"corge":{"href":"/a"},"self":{"href":"/e"}}}`,
				},
			},
			resp: &cache.Representation{
				Body: []byte(`{"_embedded":{"foo":{"_embedded":{"bar":{"_embedded":{"baz":{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}},"_links":{"baz":{"href":"/a"},"self":{"href":"/c"}}},"qux":{"_embedded":{"quux":{"_links":{"corge":{"href":"/a"},"self":{"href":"/e"}}}},"_links":{"quux":{"href":"/e"},"self":{"href":"/d"}}}},"_links":{"bar":{"href":"/c"},"qux":{"href":"/d"},"self":{"href":"/b"}}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`),
			},
		},
		{ // even With header fields do.
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/a",
				},
				Header: http.Header{
					"With": []string{"foo.bar.baz", "foo.qux.quux"},
				},
			},
			resources: map[string]*testResource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
				"/b": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"bar":{"href":"/c"},"qux":{"href":"/d"},"self":{"href":"/b"}}}`,
				},
				"/c": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"baz":{"href":"/a"},"self":{"href":"/c"}}}`,
				},
				"/d": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"quux":{"href":"/e"},"self":{"href":"/d"}}}`,
				},
				"/e": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"corge":{"href":"/a"},"self":{"href":"/e"}}}`,
				},
			},
			resp: &cache.Representation{
				Body: []byte(`{"_embedded":{"foo":{"_embedded":{"bar":{"_embedded":{"baz":{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}},"_links":{"baz":{"href":"/a"},"self":{"href":"/c"}}},"qux":{"_embedded":{"quux":{"_links":{"corge":{"href":"/a"},"self":{"href":"/e"}}}},"_links":{"quux":{"href":"/e"},"self":{"href":"/d"}}}},"_links":{"bar":{"href":"/c"},"qux":{"href":"/d"},"self":{"href":"/b"}}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`),
			},
		},
		{ // or mixture of query string and With header field.
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path:     "/a",
					RawQuery: "with=foo.bar.baz",
				},
				Header: http.Header{
					"With": []string{"foo.qux.quux"},
				},
			},
			resources: map[string]*testResource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
				"/b": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"bar":{"href":"/c"},"qux":{"href":"/d"},"self":{"href":"/b"}}}`,
				},
				"/c": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"baz":{"href":"/a"},"self":{"href":"/c"}}}`,
				},
				"/d": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"quux":{"href":"/e"},"self":{"href":"/d"}}}`,
				},
				"/e": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"corge":{"href":"/a"},"self":{"href":"/e"}}}`,
				},
			},
			resp: &cache.Representation{
				Body: []byte(`{"_embedded":{"foo":{"_embedded":{"bar":{"_embedded":{"baz":{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}},"_links":{"baz":{"href":"/a"},"self":{"href":"/c"}}},"qux":{"_embedded":{"quux":{"_links":{"corge":{"href":"/a"},"self":{"href":"/e"}}}},"_links":{"quux":{"href":"/e"},"self":{"href":"/d"}}}},"_links":{"bar":{"href":"/c"},"qux":{"href":"/d"},"self":{"href":"/b"}}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`),
			},
		},
		{ // if the response is not JSON, it simply returns the response.
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path:     "/a",
					RawQuery: "with=foo.bar.baz",
				},
			},
			resources: map[string]*testResource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/xml"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
			},
			resp: &cache.Representation{
				Body: []byte(`{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`),
			},
		},
		{ // if the specified edge is not found, it embeds a corresponding error document JSON.
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path:     "/a",
					RawQuery: "with=foo",
				},
			},
			resources: map[string]*testResource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
			},
			resp: &cache.Representation{
				Body: []byte(`{"_embedded":{"foo":{"type":"https://ichiban.github.io/jesi/problems/response-error","title":"Response Error","status":404,"detail":"Not Found","_links":{"about":"/b"}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`),
			},
		},
		{ // the resulting Cache-Control is the weakest of all.
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path:     "/a",
					RawQuery: "with=foo.bar.baz",
				},
			},
			resources: map[string]*testResource{
				"/a": {
					header: http.Header{
						"Content-Type":  []string{"application/json"},
						"Cache-Control": []string{"public,max-age=30"},
					},
					body: `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
				"/b": {
					header: http.Header{
						"Content-Type":  []string{"application/json"},
						"Cache-Control": []string{"private,max-age=20"},
					},
					body: `{"_links":{"bar":{"href":"/c"},"self":{"href":"/b"}}}`,
				},
				"/c": {
					header: http.Header{
						"Content-Type":  []string{"application/json"},
						"Cache-Control": []string{"public,max-age=10"},
					},
					body: `{"_links":{"next":{"href":"/a"},"self":{"href":"/c"}}}`,
				},
			},
			resp: &cache.Representation{
				HeaderMap: http.Header{
					"Content-Type":  []string{"application/json"},
					"Cache-Control": []string{"private,max-age=10"},
				},
				Body: []byte(`{"_embedded":{"foo":{"_embedded":{"bar":{"_embedded":{},"_links":{"next":{"href":"/a"},"self":{"href":"/c"}}}},"_links":{"bar":{"href":"/c"},"self":{"href":"/b"}}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`),
			},
		},
	}

	for i, tc := range testCases {
		th := &testHandler{
			T:         t,
			Resources: tc.resources,
		}
		e := Handler{Next: th}

		var rep cache.Representation
		e.ServeHTTP(&rep, tc.req)

		if http.StatusOK != rep.StatusCode {
			t.Errorf("(%d) expected 200, got %d, %s", i, rep.StatusCode, tc.req.URL)
		}

		for k, vs := range tc.resp.HeaderMap {
			for i, v := range vs {
				if v != rep.HeaderMap[k][i] {
					t.Errorf("(%d) (%s) expected %s, got %s", i, k, v, rep.HeaderMap[k][i])
				}

			}
		}

		if string(tc.resp.Body) != string(rep.Body) {
			t.Errorf("(%d) expected: %s, got: %s", i, string(tc.resp.Body), string(rep.Body))
		}
	}
}

type testHandler struct {
	T         *testing.T
	Resources map[string]*testResource
}

var _ http.Handler = (*testHandler)(nil)

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.T.Errorf("method is not GET: %s", r.Method)
	}

	resource, ok := h.Resources[r.URL.String()]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	header := w.Header()
	for k, v := range resource.header {
		header[k] = v
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(resource.body))
}

type testResource struct {
	header http.Header
	body   string
}
