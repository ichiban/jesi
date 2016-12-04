package cache

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTransport_RoundTrip(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		transport *Transport
		req       *http.Request
		resp      *http.Response
		cached    bool
	}{
		{ // fetch from backend and cache
			transport: &Transport{
				RoundTripper: &testTransport{
					Resources: map[string]*http.Response{
						"http://www.example.com/test": {
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Cache-Control": []string{"public"},
							},
							Body: ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Cache-Control": []string{"public"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			cached: true,
		},
		{ // fetch from backend and don't cache
			transport: &Transport{
				RoundTripper: &testTransport{
					Resources: map[string]*http.Response{
						"http://www.example.com/test": {
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Cache-Control": []string{"private"},
							},
							Body: ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Cache-Control": []string{"private"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			cached: false,
		},
		{ // fetch from cache
			transport: &Transport{
				RoundTripper: &testTransport{},
				Cache: Cache{
					Primary: map[PrimaryKey]*PrimaryEntry{
						PrimaryKey{Host: "www.example.com", Path: "/test"}: {
							Secondary: map[SecondaryKey]*SecondaryEntry{
								"": {
									Body: []byte(`{"foo":"bar"}`),
								},
							},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			cached: true,
		},
	}

	for _, tc := range testCases {
		resp, err := tc.transport.RoundTrip(tc.req)
		if err != nil {
			t.Error(err)
		}

		if tc.resp.StatusCode != resp.StatusCode {
			t.Errorf("expected %d, got %d", tc.resp.StatusCode, resp.StatusCode)
		}

		for k := range tc.resp.Header {
			if len(tc.resp.Header[k]) != len(resp.Header[k]) {
				t.Errorf("expected %d, got %d", len(tc.resp.Header), len(resp.Header))
			}
			for i := range tc.resp.Header[k] {
				if tc.resp.Header[k][i] != resp.Header[k][i] {
					t.Errorf("for header %s, expected %#v, got %#v", k, tc.resp.Header[k][i], resp.Header[k][i])
				}
			}
		}

		expected, err := ioutil.ReadAll(tc.resp.Body)
		if err != nil {
			t.Error(err)
		}

		actual, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Error(err)
		}

		if string(expected) != string(actual) {
			t.Errorf("expected %#v, got %#v", string(expected), string(actual))
		}

		cached := tc.transport.Get(tc.req)
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
		resp   *http.Response
		result bool
	}{
		{ // Non-GET requests are not cacheable.
			req: &http.Request{
				Method: http.MethodPost,
				URL:    url,
				Header: http.Header{},
				Body:   ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			resp: &http.Response{
				StatusCode: http.StatusCreated,
				Header: http.Header{
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
			resp: &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{},
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
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			result: false,
		},
		{ // "no-store" responses are not cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{
					"Cache-Control": []string{"no-store"},
				},
				Body:       ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			result: false,
		},
		{ // "private" responses are not cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{
					"Cache-Control": []string{"private"},
				},
				Body:       ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
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
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
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
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{
					"Cache-Control": []string{"public"},
				},
				Body:       ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			result: true,
		},
		{ // Responses with Expires header are cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Expires": []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			result: true,
		},
		{ // Responses with "max-age" are cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Cache-Control": []string{"max-age=600"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			result: true,
		},
		{ // Responses with "s-maxage" are cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Cache-Control": []string{"s-maxage=600"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
			},
			result: true,
		},
		{ // Responses with "public" are cacheable.
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Cache-Control": []string{"public"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"foo":"bar"}`)),
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

type testTransport struct {
	Resources map[string]*http.Response
}

var _ http.RoundTripper = (*testTransport)(nil)

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		panic(req.Method)
	}

	resp, ok := t.Resources[req.URL.String()]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       ioutil.NopCloser(strings.NewReader("")),
		}
		return resp, nil
	}

	return resp, nil
}
