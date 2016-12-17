package cache

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
)

func TestCache_Get(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Error(err)
	}

	testCases := []struct {
		cache *Cache
		req   *http.Request

		resp *CachedResponse
	}{
		{ // when it's not cached
			cache: &Cache{},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},

			resp: nil,
		},
		{ // when it's cached
			cache: &Cache{
				Primary: map[PrimaryKey]*PrimaryEntry{
					PrimaryKey{Host: "www.example.com", Path: "/test"}: {
						Secondary: map[SecondaryKey]*CachedResponse{
							"": {
								Body: []byte(`{"foo":"bar"}`),
							},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},

			resp: &CachedResponse{
				Header: http.Header{},
				Body:   []byte(`{"foo":"bar"}`),
			},
		},
		{ // when it's cached and also the secondary key matches
			cache: &Cache{
				Primary: map[PrimaryKey]*PrimaryEntry{
					PrimaryKey{Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						Secondary: map[SecondaryKey]*CachedResponse{
							"Accept=application%2Fjson&Accept-Language=ja-JP": {
								Body: []byte(`{"foo":"bar"}`),
							},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{
					"Accept":          []string{"application/json"},
					"Accept-Language": []string{"ja-JP"},
				},
			},

			resp: &CachedResponse{
				Header: http.Header{},
				Body:   []byte(`{"foo":"bar"}`),
			},
		},
		{ // when it's cached but the secondary key doesn't match
			cache: &Cache{
				Primary: map[PrimaryKey]*PrimaryEntry{
					PrimaryKey{Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						Secondary: map[SecondaryKey]*CachedResponse{
							"Accept=application%2Fjson&Accept-Language=ja-JP": {
								Body: []byte(`{"foo":"bar"}`),
							},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
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

		if string(tc.resp.Body) != string(resp.Body) {
			t.Errorf("expected %#v, got %#v", string(tc.resp.Body), string(resp.Body))
		}
	}
}

func TestCache_Set(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Error(err)
	}

	testCases := []struct {
		cache *Cache
		req   *http.Request
		resp  *CachedResponse

		primary   PrimaryKey
		secondary SecondaryKey
	}{
		{ // when there's no entry for the request (insert)
			cache: &Cache{},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},
			resp: &CachedResponse{
				Body: []byte{},
			},

			primary:   PrimaryKey{Host: "www.example.com", Path: "/test"},
			secondary: "",
		},
		{ // when there's an existing entry for the request (replace)
			cache: &Cache{
				Primary: map[PrimaryKey]*PrimaryEntry{
					PrimaryKey{Host: "www.example.com", Path: "/test"}: {
						Secondary: map[SecondaryKey]*CachedResponse{
							"": {},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},
			resp: &CachedResponse{
				Body: []byte{},
			},

			primary:   PrimaryKey{Host: "www.example.com", Path: "/test"},
			secondary: "",
		},
		{ // when there's Vary header field
			cache: &Cache{},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{
					"Accept":          []string{"application/json"},
					"Accept-Language": []string{"ja-JP"},
				},
			},
			resp: &CachedResponse{
				Body: []byte{},
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

		if http.StatusOK != resp.StatusCode {
			t.Errorf("expected %d, got %d", http.StatusOK, resp.StatusCode)
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

		actual, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Error(err)
		}

		if string(tc.resp.Body) != string(actual) {
			t.Errorf("expected %#v, got %#v", string(tc.resp.Body), string(actual))
		}

		if tc.cache.History.Len() != 1 {
			t.Errorf("expected 1, got %d", tc.cache.History.Len())
		}

		k := tc.cache.History.Front().Value.(key)
		if tc.primary != k.primary  {
			t.Errorf("expected %v, got %v", tc.primary, k.primary)
		}
		if tc.secondary != k.secondary  {
			t.Errorf("expected %v, got %v", tc.primary, k.primary)
		}
	}
}
