package conditional

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTransport_RoundTrip(t *testing.T) {
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

	transport := &Transport{
		RoundTripper: &testTransport{},
	}

	for i, tc := range testCases {
		resp, err := transport.RoundTrip(tc.req)
		if err != nil {
			panic(err)
		}

		if tc.resp.StatusCode != resp.StatusCode {
			t.Errorf("(%d) expected: %d, got: %d", i, tc.resp.StatusCode, resp.StatusCode)
		}

		for k, vs := range resp.Header {
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

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}

		if string(tcb) != string(b) {
			t.Errorf("(%d) expected: %s, got: %s", i, string(tcb), string(b))
		}
	}
}

type testTransport struct{}

var _ http.RoundTripper = (*testTransport)(nil)

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/foo":
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Etag": []string{`W/"foo"`},
			},
			Body: ioutil.NopCloser(strings.NewReader("foo")),
		}, nil
	case "/bar":
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       ioutil.NopCloser(strings.NewReader("bar")),
		}, nil
	default:
		return &http.Response{
			StatusCode: http.StatusNotFound,
		}, nil
	}
}
