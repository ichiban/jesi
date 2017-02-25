package conditional

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ichiban/jesi/common"
)

func TestHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		req  *http.Request
		resp *http.Response
	}{
		{ // POST requests
			req: &http.Request{
				Method: http.MethodPost,
				URL: &url.URL{
					Path: "/foo",
				},
				Header: http.Header{},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Etag": []string{`W/"foo"`},
				},
				Body: ioutil.NopCloser(strings.NewReader("foo")),
			},
		},
		{ // GET requests without If-None-Match header
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/foo",
				},
				Header: http.Header{},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Etag": []string{`W/"foo"`},
				},
				Body: ioutil.NopCloser(strings.NewReader("foo")),
			},
		},
		{ // GET requests with If-None-Match header
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/foo",
				},
				Header: http.Header{
					"If-None-Match": []string{`W/"foo"`},
				},
			},
			resp: &http.Response{
				StatusCode: http.StatusNotModified,
				Header: http.Header{
					"Etag": []string{`W/"foo"`},
				},
				Body: ioutil.NopCloser(strings.NewReader("")),
			},
		},
		{ // GET requests with If-None-Match header but the response doesn't have ETag
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/bar",
				},
				Header: http.Header{
					"If-None-Match": []string{`W/"bar"`},
				},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader("bar")),
			},
		},
	}

	handler := &Handler{
		Next: &testHandler{},
	}

	for i, tc := range testCases {
		var resp common.ResponseBuffer
		handler.ServeHTTP(&resp, tc.req)

		if tc.resp.StatusCode != resp.StatusCode {
			t.Errorf("(%d) expected: %d, got: %d", i, tc.resp.StatusCode, resp.StatusCode)
		}

		for k, vs := range resp.HeaderMap {
			for j, v := range vs {
				if tc.resp.Header[k][j] != v {
					t.Errorf("(%d) [%s] expected: %s, got: %s", i, k, tc.resp.Header[k][j], v)
				}
			}
		}

		tcb, err := ioutil.ReadAll(tc.resp.Body)
		if err != nil {
			panic(err)
		}

		if string(tcb) != string(resp.Body) {
			t.Errorf("(%d) expected: %s, got: %s", i, string(tcb), string(resp.Body))
		}
	}
}

type testHandler struct{}

var _ http.Handler = (*testHandler)(nil)

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/foo":
		w.Header()["Etag"] = []string{`W/"foo"`}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("foo"))
	case "/bar":
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("bar"))
	default:
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte{})
	}
}
