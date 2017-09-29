package balance

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ichiban/jesi/cache"
)

func TestHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		node       *Node
		backends   []*Backend
		givenReqs  []*http.Request
		givenResps []*http.Response

		expectedReqs  []*http.Request
		expectedResps []*http.Response
	}{
		{ // if there's no backend available, it returns 502 Bad Gateway.
			backends: []*Backend{},
			givenReqs: []*http.Request{
				{
					Method: http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
				},
			},
			givenResps: []*http.Response{
				nil,
			},

			expectedReqs: []*http.Request{
				nil,
			},
			expectedResps: []*http.Response{
				{
					StatusCode: http.StatusBadGateway,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("")),
				},
			},
		},
		{ // if there're multiple backends available, it spreads the workload across them.
			backends: []*Backend{
				{URL: &url.URL{Scheme: "https", Host: "a.example.com"}},
				{URL: &url.URL{Scheme: "https", Host: "b.example.com"}},
				{URL: &url.URL{Scheme: "https", Host: "c.example.com"}},
			},
			givenReqs: []*http.Request{
				{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}},
				{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}},
				{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}},
				{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}},
				{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}},
				{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}},
			},
			givenResps: []*http.Response{
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
			},

			expectedReqs: []*http.Request{
				{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "a.example.com", Path: "/foo"}, Header: http.Header{"X-Forwarded-Proto": []string{"http"}, "Forwarded": []string{`proto=http`}}},
				{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "b.example.com", Path: "/foo"}, Header: http.Header{"X-Forwarded-Proto": []string{"http"}, "Forwarded": []string{`proto=http`}}},
				{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "c.example.com", Path: "/foo"}, Header: http.Header{"X-Forwarded-Proto": []string{"http"}, "Forwarded": []string{`proto=http`}}},
				{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "a.example.com", Path: "/foo"}, Header: http.Header{"X-Forwarded-Proto": []string{"http"}, "Forwarded": []string{`proto=http`}}},
				{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "b.example.com", Path: "/foo"}, Header: http.Header{"X-Forwarded-Proto": []string{"http"}, "Forwarded": []string{`proto=http`}}},
				{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "c.example.com", Path: "/foo"}, Header: http.Header{"X-Forwarded-Proto": []string{"http"}, "Forwarded": []string{`proto=http`}}},
			},
			expectedResps: []*http.Response{
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
				{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("foo"))},
			},
		},
		{ // with hop-by-hop headers
			backends: []*Backend{
				{URL: &url.URL{Scheme: "https", Host: "a.example.com"}},
			},
			givenReqs: []*http.Request{
				{
					Method: http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"Connection": []string{"foo, bar"},
						"Foo":        []string{"abc"},
						"Bar":        []string{"def"},
						"Baz":        []string{"ghq"},
					},
				},
			},
			givenResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Connection": []string{"hoge, foo"},
						"Hoge":       []string{"abc"},
						"Fuga":       []string{"def"},
						"Foo":        []string{"ghq"},
					},
					Body: ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
			expectedReqs: []*http.Request{
				{
					Method: http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"Baz":               []string{"ghq"},
						"X-Forwarded-Proto": []string{"http"},
						"Forwarded":         []string{`proto=http`},
					},
				},
			},
			expectedResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Fuga": []string{"def"},
					},
					Body: ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
		},
		{ // with Node, RemoteAddr, Host, and TLS
			node: &Node{
				ID: "_d3d2e741-5d38-4356-8df4-b1d019bd634e",
			},
			backends: []*Backend{
				{URL: &url.URL{Scheme: "https", Host: "a.example.com"}},
			},
			givenReqs: []*http.Request{
				{
					RemoteAddr: "192.0.2.1:12345",
					Host:       "example.com",
					Method:     http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{},
					TLS:    &tls.ConnectionState{},
				},
			},
			givenResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
			expectedReqs: []*http.Request{
				{
					Method: http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"X-Forwarded-For":   []string{"192.0.2.1"},
						"X-Forwarded-Host":  []string{"example.com"},
						"X-Forwarded-Proto": []string{"https"},
						"Forwarded":         []string{`by=_d3d2e741-5d38-4356-8df4-b1d019bd634e;for="192.0.2.1:12345";host=example.com;proto=https`},
					},
				},
			},
			expectedResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
		},
		{ // with RemoteAddr, X-Forwarded-For, and Forwarded
			backends: []*Backend{
				{URL: &url.URL{Scheme: "https", Host: "a.example.com"}},
			},
			givenReqs: []*http.Request{
				{
					RemoteAddr: "192.0.2.3:12345",
					Method:     http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"X-Forwarded-For":   []string{"192.0.2.1, 192.0.2.2"},
						"X-Forwarded-Proto": []string{"http"},
						"Forwarded":         []string{`for="192.0.2.1:12345"`, `for="192.0.2.2:12345"`},
					},
				},
			},
			givenResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
			expectedReqs: []*http.Request{
				{
					Method: http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"X-Forwarded-For":   []string{"192.0.2.1, 192.0.2.2, 192.0.2.3"},
						"X-Forwarded-Proto": []string{"http"},
						"Forwarded":         []string{`for="192.0.2.1:12345"`, `for="192.0.2.2:12345"`, `for="192.0.2.3:12345";proto=http`},
					},
				},
			},
			expectedResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
		},
		{ // with RemoteAddr, X-Forwarded-For (X-Forwarded-For will be converted to Forwarded)
			backends: []*Backend{
				{URL: &url.URL{Scheme: "https", Host: "a.example.com"}},
			},
			givenReqs: []*http.Request{
				{
					RemoteAddr: "192.0.2.3:12345",
					Method:     http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"X-Forwarded-For": []string{"192.0.2.1, 192.0.2.2"},
					},
				},
			},
			givenResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
			expectedReqs: []*http.Request{
				{
					Method: http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"X-Forwarded-For":   []string{"192.0.2.1, 192.0.2.2, 192.0.2.3"},
						"X-Forwarded-Proto": []string{"http"},
						"Forwarded":         []string{`for=192.0.2.1`, `for=192.0.2.2`, `for="192.0.2.3:12345";proto=http`},
					},
				},
			},
			expectedResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
		},
		{ // with RemoteAddr, X-Forwarded-For, and X-Forwarded-By
			backends: []*Backend{
				{URL: &url.URL{Scheme: "https", Host: "a.example.com"}},
			},
			givenReqs: []*http.Request{
				{
					RemoteAddr: "192.0.2.3:12345",
					Method:     http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"X-Forwarded-By":  []string{"192.0.2.10"},
						"X-Forwarded-For": []string{"192.0.2.1, 192.0.2.2"},
					},
				},
			},
			givenResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
			expectedReqs: []*http.Request{
				{
					Method: http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"X-Forwarded-By":    []string{"192.0.2.10"},
						"X-Forwarded-For":   []string{"192.0.2.1, 192.0.2.2, 192.0.2.3"},
						"X-Forwarded-Proto": []string{"http"},
						"Forwarded":         []string{`for=192.0.2.1`, `for=192.0.2.2`, `for="192.0.2.3:12345";proto=http`},
					},
				},
			},
			expectedResps: []*http.Response{
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("foo")),
				},
			},
		},
		{ // with error
			backends: []*Backend{
				{URL: &url.URL{Scheme: "https", Host: "a.example.com"}},
			},
			givenReqs: []*http.Request{
				{
					Method: http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{},
				},
			},
			givenResps: []*http.Response{
				{
					Body: ioutil.NopCloser(strings.NewReader("")),
				},
			},
			expectedReqs: []*http.Request{
				{
					Method: http.MethodGet,
					URL: &url.URL{
						Path: "/foo",
					},
					Header: http.Header{
						"X-Forwarded-Proto": []string{"http"},
						"Forwarded":         []string{`proto=http`},
					},
				},
			},
			expectedResps: []*http.Response{
				{
					StatusCode: http.StatusBadGateway,
					Header:     http.Header{},
					Body:       ioutil.NopCloser(strings.NewReader("")),
				},
			},
		},
	}

	for i, tc := range testCases {
		var p BackendPool
		for _, b := range tc.backends {
			b.Client = http.Client{Transport: &testRoundTripper{statuses: []int{http.StatusOK}}}
			p.Add(b)
		}

		for n := range tc.givenReqs {
			h := &testHandler{
				resp: tc.givenResps[n],
			}

			handler := &Handler{
				Node:        tc.node,
				BackendPool: &p,
				Next:        h,
			}

			var rep cache.Representation
			handler.ServeHTTP(&rep, tc.givenReqs[n])

			// request
			if h.req != nil {
				if len(tc.expectedReqs[n].Header) != len(h.req.Header) {
					t.Errorf("(%d) expected: %v, got: %v", i, tc.expectedReqs[n].Header, h.req.Header)
				}
				for k, vs := range h.req.Header {
					if len(tc.expectedReqs[n].Header[k]) != len(vs) {
						t.Errorf("(%d) [%s] expected: %d, got: %d", i, k, len(tc.expectedReqs[n].Header[k]), len(vs))
						continue
					}
					for j, v := range vs {
						if tc.expectedReqs[n].Header[k][j] != v {
							t.Errorf("(%d) [%s] expected: %s, got: %s", i, k, tc.expectedReqs[n].Header[k][j], v)
						}
					}
				}
			}

			// response
			if len(tc.expectedResps[n].Header) != len(rep.HeaderMap) {
				t.Errorf("(%d) expected: %d, got: %d", i, len(tc.expectedResps[n].Header), len(rep.HeaderMap))
			}
			for k, vs := range rep.HeaderMap {
				if len(tc.expectedResps[n].Header[k]) != len(vs) {
					t.Errorf("(%d) [%s] expected: %d, got: %d", i, k, len(tc.expectedResps[n].Header[k]), len(vs))
					continue
				}
				for j, v := range vs {
					if tc.expectedResps[n].Header[k][j] != v {
						t.Errorf("(%d) [%s] expected: %s, got: %s", i, k, tc.expectedResps[n].Header[k][j], v)
					}
				}
			}

			if tc.expectedResps[n].Body == nil {
				continue
			}

			tcb, err := ioutil.ReadAll(tc.expectedResps[n].Body)
			if err != nil {
				panic(err)
			}

			if string(tcb) != string(rep.Body) {
				t.Errorf("(%d) expected: %s, got: %s", i, string(tcb), string(rep.Body))
			}
		}
	}
}

type testHandler struct {
	req  *http.Request
	resp *http.Response
}

var _ http.Handler = (*testHandler)(nil)

func (t *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.req = r
	for k, vs := range t.resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(t.resp.StatusCode)
	io.Copy(w, t.resp.Body)
}
