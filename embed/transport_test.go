package embed

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

func TestTransport_RoundTrip(t *testing.T) {
	testCases := []struct {
		url       string
		resources map[string]*resource
		body      string
	}{
		{ // without 'with' query parameter, it simply returns JSON.
			url: "/test",
			resources: map[string]*resource{
				"/test": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{}`,
				},
			},
			body: `{}`,
		},
		{ // with 'with' query parameter, it embeds resources specified by edges.
			url: "/a?with=foo.bar.baz",
			resources: map[string]*resource{
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
			resources: map[string]*resource{
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
			resources: map[string]*resource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/xml"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
			},
			body: `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
		},
		{ // if the specified edge is not found, it embeds a corresponding error document JSON.
			url: "/a?with=foo",
			resources: map[string]*resource{
				"/a": {
					header: http.Header{"Content-Type": []string{"application/json"}},
					body:   `{"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
				},
			},
			body: `{"_embedded":{"errors":[{"status":404,"title":"Error Response","detail":"Not Found","_links":{"about":"/b"}}]},"_links":{"foo":{"href":"/b"},"self":{"href":"/a"}}}`,
		},
	}

	for _, tc := range testCases {
		req, err := http.NewRequest(http.MethodGet, tc.url, bytes.NewReader([]byte{}))
		if err != nil {
			t.Errorf("err is not nil: %v", err)
		}

		tt := &testTransport{
			T:         t,
			Resources: tc.resources,
		}
		e := Transport{tt}

		r, err := e.RoundTrip(req)
		if err != nil {
			t.Errorf("err is not nil: %v", err)
		}

		if http.StatusOK != r.StatusCode {
			t.Errorf("expected 200, got %d, %s", r.StatusCode, req.URL)
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Errorf("err is not nil: %v", err)
		}

		if tc.body != string(body) {
			t.Errorf("expected: %s, got: %s", tc.body, string(body))
		}
	}
}

type testTransport struct {
	T         *testing.T
	Resources map[string]*resource
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

type resource struct {
	header http.Header
	body   string
}
