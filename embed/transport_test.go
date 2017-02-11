package embed

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

func TestTransport_RoundTrip(t *testing.T) {
	testCases := []struct {
		url       string
		resources map[string]*testResource
		header    http.Header
		body      string
	}{
		{ // without 'with' query parameter, it simply returns JSON.
			url: "/test",
			resources: map[string]*testResource{
				"/test": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{}`,
				},
			},
			body: `{}`,
		},
		{ // with 'with' query parameter, it embeds resources specified by edges.
			url: "/a?with=foo.bar.baz",
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
			body: `{"_embedded":{"foo":{"_embedded":{"bar":{"_embedded":{},"_links":{"next":{"href":"/a"},"self":{"href":"/c"}}}},"_links":{"bar":{"href":"/c"},"self":{"href":"/b"}}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
		},
		{ // multiple 'with' query parameters are also fine.
			url: "/a?with=foo.bar.baz&with=foo.qux.quux",
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
			body: `{"_embedded":{"foo":{"_embedded":{"bar":{"_embedded":{"baz":{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}},"_links":{"baz":{"href":"/a"},"self":{"href":"/c"}}},"qux":{"_embedded":{"quux":{"_links":{"corge":{"href":"/a"},"self":{"href":"/e"}}}},"_links":{"quux":{"href":"/e"},"self":{"href":"/d"}}}},"_links":{"bar":{"href":"/c"},"qux":{"href":"/d"},"self":{"href":"/b"}}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
		},
		{ // if the response is not JSON, it simply returns the response.
			url: "/a?with=foo.bar.baz",
			resources: map[string]*testResource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/xml"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
			},
			body: `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
		},
		{ // if the specified edge is not found, it embeds a corresponding error document JSON.
			url: "/a?with=foo",
			resources: map[string]*testResource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
			},
			body: `{"_embedded":{"foo":{"type":"https://ichiban.github.io/jesi/problems/response-error","title":"Response Error","status":404,"detail":"Not Found","_links":{"about":"/b"}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
		},
		{ // the resulting Cache-Control is the weakest of all.
			url: "/a?with=foo.bar.baz",
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
			header: http.Header{
				"Content-Type":  []string{"application/json"},
				"Cache-Control": []string{"private,max-age=10"},
			},
			body: `{"_embedded":{"foo":{"_embedded":{"bar":{"_embedded":{},"_links":{"next":{"href":"/a"},"self":{"href":"/c"}}}},"_links":{"bar":{"href":"/c"},"self":{"href":"/b"}}}},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
		},
	}

	for i, tc := range testCases {
		req, err := http.NewRequest(http.MethodGet, tc.url, nil)
		if err != nil {
			t.Errorf("(%d) err is not nil: %v", i, err)
		}

		tt := &testTransport{
			T:         t,
			Resources: tc.resources,
		}
		e := Transport{tt}

		r, err := e.RoundTrip(req)
		if err != nil {
			t.Errorf("(%d) err is not nil: %v", i, err)
		}

		if http.StatusOK != r.StatusCode {
			t.Errorf("(%d) expected 200, got %d, %s", i, r.StatusCode, req.URL)
		}

		for k, vs := range tc.header {
			for i, v := range vs {
				if v != r.Header[k][i] {
					t.Errorf("(%d) (%s) expected %s, got %s", i, k, v, r.Header[k][i])
				}

			}
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Errorf("(%d) err is not nil: %v", i, err)
		}

		if tc.body != string(body) {
			t.Errorf("(%d) expected: %s, got: %s", i, tc.body, string(body))
		}
	}
}

type testTransport struct {
	T         *testing.T
	Resources map[string]*testResource
}

var _ http.RoundTripper = (*testTransport)(nil)

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		t.T.Errorf("method is not GET: %s", req.Method)
	}

	resource, ok := t.Resources[req.URL.String()]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{},
			Body:       ioutil.NopCloser(strings.NewReader("")),
		}
		return resp, nil
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     resource.header,
		Body:       ioutil.NopCloser(strings.NewReader(resource.body)),
	}

	return resp, nil
}

type testResource struct {
	header http.Header
	body   string
}
