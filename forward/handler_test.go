package forward

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

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
		{ // success
			givenReq:     &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}, Header: http.Header{"Foo": []string{"abc"}, "Bar": []string{"def"}}},
			givenResp:    &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Hoge": []string{"abc"}, "Fuga": []string{"def"}}, Body: ioutil.NopCloser(strings.NewReader("foo"))},
			expectedReq:  &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}, Header: http.Header{"Foo": []string{"abc"}, "Bar": []string{"def"}}},
			expectedResp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Hoge": []string{"abc"}, "Fuga": []string{"def"}}, Body: ioutil.NopCloser(strings.NewReader("foo"))},
		},
		{ // error
			givenReq:     &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}},
			givenErr:     errors.New("failed"),
			expectedReq:  &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/foo"}},
			expectedResp: &http.Response{StatusCode: http.StatusBadGateway, Body: ioutil.NopCloser(strings.NewReader(""))},
		},
	}

	for i, tc := range testCases {
		transport := &testTransport{
			resp: tc.givenResp,
			err:  tc.givenErr,
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
