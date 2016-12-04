package cache

import (
	"net/http"
	"testing"
	"bytes"
	"io/ioutil"
	"net/url"
)

func TestCache_Get(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Error(err)
	}

	testCases := []struct {
		cache  *Cache
		req *http.Request

		resp *http.Response
	}{
		{ // when it's not cached
			cache:  &Cache{},
			req: &http.Request{
				Method: http.MethodGet,
				URL: url,
			},

			resp: nil,
		},
		{ // when it's cached
			cache: &Cache{
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
			req: &http.Request{
				Method: http.MethodGet,
				URL: url,
			},

			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{},
				Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"foo":"bar"}`))),
			},
		},
		{ // when it's cached and also the secondary key matches
			cache: &Cache{
				Primary: map[PrimaryKey]*PrimaryEntry{
					PrimaryKey{Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						Secondary: map[SecondaryKey]*SecondaryEntry{
							"Accept=application%2Fjson&Accept-Language=ja-JP": {
								Body: []byte(`{"foo":"bar"}`),
							},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL: url,
				Header: http.Header{
					"Accept":          []string{"application/json"},
					"Accept-Language": []string{"ja-JP"},
				},
			},

			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{},
				Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"foo":"bar"}`))),
			},
		},
		{ // when it's cached but the secondary key doesn't match
			cache: &Cache{
				Primary: map[PrimaryKey]*PrimaryEntry{
					PrimaryKey{Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						Secondary: map[SecondaryKey]*SecondaryEntry{
							"Accept=application%2Fjson&Accept-Language=ja-JP": {
								Body: []byte(`{"foo":"bar"}`),
							},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL: url,
				Header: http.Header{
					"Accept":          []string{"application/json"},
					"Accept-Language": []string{"en-US"},
				},
			},

			resp: nil,
		},
	}

	for _, tc := range testCases {
		resp := tc.cache.Get(tc.req)

		if tc.resp == nil && resp != nil {
			t.Errorf("expected nil, got %#v", resp)
			continue
		}

		if tc.resp != nil && resp == nil {
			t.Error("expected non-nil, got nil")
			continue
		}

		if tc.resp == nil && resp == nil {
			continue
		}

		if tc.resp.StatusCode != resp.StatusCode {
			t.Errorf("expected %d, got %d", tc.resp.StatusCode, resp.StatusCode)
		}

		for k := range tc.resp.Header {
			if len(tc.resp.Header) != len(resp.Header) {
				t.Errorf("expected %d, got %d", len(tc.resp.Header), len(resp.Header))
			}
			for i := 0; i < len(tc.resp.Header); i++ {
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
			t.Errorf("expected %#v, got %#v", expected, actual)
		}
	}
}

func TestCache_Set(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Error(err)
	}

	testCases := []struct {
		cache     *Cache
		req *http.Request
		resp      *http.Response

		primary   PrimaryKey
		secondary SecondaryKey
	}{
		{ // when there's no entry for the request (insert)
			cache:  &Cache{},
			req: &http.Request{
				Method: http.MethodGet,
				URL: url,
			},
			resp:   &http.Response{
				StatusCode: http.StatusOK,
				Body: ioutil.NopCloser(bytes.NewReader([]byte{})),
			},

			primary:   PrimaryKey{Host: "www.example.com", Path: "/test"},
			secondary: "",
		},
		{ // when there's an existing entry for the request (replace)
			cache: &Cache{
				Primary: map[PrimaryKey]*PrimaryEntry{
					PrimaryKey{Host: "www.example.com", Path: "/test"}: {
						Secondary: map[SecondaryKey]*SecondaryEntry{
							"": {},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL: url,
			},
			resp:   &http.Response{
				StatusCode: http.StatusOK,
				Body: ioutil.NopCloser(bytes.NewReader([]byte{})),
			},

			primary:   PrimaryKey{Host: "www.example.com", Path: "/test"},
			secondary: "",
		},
		{ // when there's Vary header field
			cache: &Cache{},
			req: &http.Request{
				Method: http.MethodGet,
				URL: url,
				Header: http.Header{
					"Accept":          []string{"application/json"},
					"Accept-Language": []string{"ja-JP"},
				},
			},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Body: ioutil.NopCloser(bytes.NewReader([]byte{})),
				Header: http.Header{
					"Vary": []string{"Accept", "Accept-Language"},
				},
			},

			primary:   PrimaryKey{Host: "www.example.com", Path: "/test"},
			secondary: "Accept=application%2Fjson&Accept-Language=ja-JP",
		},
	}

	for _, tc := range testCases {
		tc.cache.Set(tc.req, tc.resp)

		pe, ok := tc.cache.Primary[tc.primary]
		if !ok {
			t.Errorf("expected cache to have an entry for %#v but not", tc.primary)
		}

		result, ok := pe.Secondary[tc.secondary]
		if !ok {
			t.Errorf("expected %#v to have a response for %#v but not", pe, tc.secondary)
		}

		resp := result.Response()

		if tc.resp.StatusCode != resp.StatusCode {
			t.Errorf("expected %d, got %d", tc.resp.StatusCode, resp.StatusCode)
		}

		for k := range tc.resp.Header {
			if len(tc.resp.Header) != len(resp.Header) {
				t.Errorf("expected %d, got %d", len(tc.resp.Header), len(resp.Header))
			}
			for i := 0; i < len(tc.resp.Header[k]); i++ {
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
			t.Errorf("expected %#v, got %#v", expected, actual)
		}
	}
}
