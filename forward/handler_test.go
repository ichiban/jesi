package forward

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"crypto/tls"
	"github.com/ichiban/jesi/cache"
)

func TestHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		givenReq  *http.Request
		givenResp *http.Response
		givenErr  error

		expectedReq  *http.Request
		expectedResp *http.Response
	}{
		{ // with hop-by-hop headers
			givenReq: &http.Request{
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
			givenResp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Connection": []string{"hoge, foo"},
					"Hoge":       []string{"abc"},
					"Fuga":       []string{"def"},
					"Foo":        []string{"ghq"},
				},
				Body: ioutil.NopCloser(strings.NewReader("foo")),
			},
			expectedReq: &http.Request{
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
			expectedResp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Fuga": []string{"def"},
				},
				Body: ioutil.NopCloser(strings.NewReader("foo")),
			},
		},
		{ // with RemoteAddr, Host, and TLS
			givenReq: &http.Request{
				RemoteAddr: "192.0.2.1:12345",
				Host:       "example.com",
				Method:     http.MethodGet,
				URL: &url.URL{
					Path: "/foo",
				},
				Header: http.Header{},
				TLS:    &tls.ConnectionState{},
			},
			givenResp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader("foo")),
			},
			expectedReq: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/foo",
				},
				Header: http.Header{
					"X-Forwarded-For":   []string{"192.0.2.1"},
					"X-Forwarded-Host":  []string{"example.com"},
					"X-Forwarded-Proto": []string{"https"},
					"Forwarded":         []string{`for="192.0.2.1:12345";host=example.com;proto=https`},
				},
			},
			expectedResp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader("foo")),
			},
		},
		{ // with RemoteAddr, X-Forwarded-For, and Forwarded
			givenReq: &http.Request{
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
			givenResp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader("foo")),
			},
			expectedReq: &http.Request{
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
			expectedResp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader("foo")),
			},
		},
		{ // with RemoteAddr, X-Forwarded-For (X-Forwarded-For will be converted to Forwarded)
			givenReq: &http.Request{
				RemoteAddr: "192.0.2.3:12345",
				Method:     http.MethodGet,
				URL: &url.URL{
					Path: "/foo",
				},
				Header: http.Header{
					"X-Forwarded-For": []string{"192.0.2.1, 192.0.2.2"},
				},
			},
			givenResp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader("foo")),
			},
			expectedReq: &http.Request{
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
			expectedResp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader("foo")),
			},
		},
		{ // with RemoteAddr, X-Forwarded-For, and X-Forwarded-By
			givenReq: &http.Request{
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
			givenResp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader("foo")),
			},
			expectedReq: &http.Request{
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
			expectedResp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader("foo")),
			},
		},
		{ // with error
			givenReq: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/foo",
				},
				Header: http.Header{},
			},
			givenResp: &http.Response{},
			givenErr:  errors.New("failed"),
			expectedReq: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/foo",
				},
				Header: http.Header{
					"X-Forwarded-Proto": []string{"http"},
					"Forwarded":         []string{`proto=http`},
				},
			},
			expectedResp: &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     http.Header{},
			},
		},
	}

	for i, tc := range testCases {
		transport := &testTransport{
			resp: tc.givenResp,
		}
		handler := &Handler{
			Transport: transport,
		}

		var rep cache.Representation
		handler.ServeHTTP(&rep, tc.givenReq)

		// request
		if len(tc.expectedReq.Header) != len(transport.req.Header) {
			t.Errorf("(%d) expected: %v, got: %v", i, tc.expectedReq.Header, transport.req.Header)
		}
		for k, vs := range transport.req.Header {
			if len(tc.expectedReq.Header[k]) != len(vs) {
				t.Errorf("(%d) [%s] expected: %d, got: %d", i, k, len(tc.expectedReq.Header[k]), len(vs))
				continue
			}
			for j, v := range vs {
				if tc.expectedReq.Header[k][j] != v {
					t.Errorf("(%d) [%s] expected: %s, got: %s", i, k, tc.expectedReq.Header[k][j], v)
				}
			}
		}

		// response
		if len(tc.expectedResp.Header) != len(rep.HeaderMap) {
			t.Errorf("(%d) expected: %d, got: %d", i, len(tc.expectedResp.Header), len(rep.HeaderMap))
		}
		for k, vs := range rep.HeaderMap {
			if len(tc.expectedResp.Header[k]) != len(vs) {
				t.Errorf("(%d) [%s] expected: %d, got: %d", i, k, len(tc.expectedResp.Header[k]), len(vs))
				continue
			}
			for j, v := range vs {
				if tc.expectedResp.Header[k][j] != v {
					t.Errorf("(%d) [%s] expected: %s, got: %s", i, k, tc.expectedResp.Header[k][j], v)
				}
			}
		}

		if tc.expectedResp.Body == nil {
			continue
		}

		tcb, err := ioutil.ReadAll(tc.expectedResp.Body)
		if err != nil {
			panic(err)
		}

		if string(tcb) != string(rep.Body) {
			t.Errorf("(%d) expected: %s, got: %s", i, string(tcb), string(rep.Body))
		}
	}
}

type testTransport struct {
	req  *http.Request
	resp *http.Response
	err  error
}

var _ http.RoundTripper = (*testTransport)(nil)

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.req = req
	return t.resp, t.err
}
