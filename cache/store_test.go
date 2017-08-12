package cache

import (
	"net/http"
	"net/url"
	"testing"
)

func TestStore_Get(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Error(err)
	}

	testCases := []struct {
		store *Store
		req   *http.Request

		rep *Representation
	}{
		{ // when it's not cached
			store: &Store{},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},

			rep: nil,
		},
		{ // when it's cached
			store: &Store{
				Resources: map[ResourceKey]*Resource{
					ResourceKey{Method: http.MethodGet, Host: "www.example.com", Path: "/test"}: {
						Representations: map[RepresentationKey]*Representation{
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

			rep: &Representation{
				HeaderMap: http.Header{},
				Body:      []byte(`{"foo":"bar"}`),
			},
		},
		{ // when it's cached and also the representation key matches
			store: &Store{
				Resources: map[ResourceKey]*Resource{
					ResourceKey{Method: http.MethodGet, Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						Representations: map[RepresentationKey]*Representation{
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

			rep: &Representation{
				HeaderMap: http.Header{},
				Body:      []byte(`{"foo":"bar"}`),
			},
		},
		{ // when it's cached but the representation key doesn't match
			store: &Store{
				Resources: map[ResourceKey]*Resource{
					ResourceKey{Method: http.MethodGet, Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						Representations: map[RepresentationKey]*Representation{
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

			rep: nil,
		},
		{ // when it's cached but the method doesn't match
			store: &Store{
				Resources: map[ResourceKey]*Resource{
					ResourceKey{Method: http.MethodGet, Host: "www.example.com", Path: "/test"}: {
						Representations: map[RepresentationKey]*Representation{
							"": {
								Body: []byte(`{"foo":"bar"}`),
							},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodHead,
				URL:    url,
			},

			rep: nil,
		},
	}

	for _, tc := range testCases {
		rep := tc.store.Get(tc.req)

		if tc.rep == nil && rep != nil {
			t.Errorf("expected nil, got %#v", rep)
			continue
		}

		if tc.rep != nil && rep == nil {
			t.Error("expected non-nil, got nil")
			continue
		}

		if tc.rep == nil && rep == nil {
			continue
		}

		for k := range tc.rep.HeaderMap {
			if len(tc.rep.HeaderMap) != len(rep.HeaderMap) {
				t.Errorf("expected %d, got %d", len(tc.rep.HeaderMap), len(rep.HeaderMap))
			}
			for i := 0; i < len(tc.rep.HeaderMap); i++ {
				if tc.rep.HeaderMap[k][i] != rep.HeaderMap[k][i] {
					t.Errorf("for header %s, expected %#v, got %#v", k, tc.rep.HeaderMap[k][i], rep.HeaderMap[k][i])
				}
			}
		}

		if string(tc.rep.Body) != string(rep.Body) {
			t.Errorf("expected %#v, got %#v", string(tc.rep.Body), string(rep.Body))
		}
	}
}

func TestStore_Set(t *testing.T) {
	url, err := url.Parse("http://www.example.com/test")
	if err != nil {
		t.Error(err)
	}

	testCases := []struct {
		store *Store
		req   *http.Request
		rep   *Representation

		primary   ResourceKey
		secondary RepresentationKey
	}{
		{ // when there's no entry for the request (insert)
			store: &Store{},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},
			rep: &Representation{
				Body: []byte{},
			},

			primary:   ResourceKey{Method: http.MethodGet, Host: "www.example.com", Path: "/test"},
			secondary: "",
		},
		{ // when there's an existing entry for the request (replace)
			store: &Store{
				Resources: map[ResourceKey]*Resource{
					ResourceKey{Method: http.MethodGet, Host: "www.example.com", Path: "/test"}: {
						Representations: map[RepresentationKey]*Representation{
							"": {},
						},
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},
			rep: &Representation{
				Body: []byte{},
			},

			primary:   ResourceKey{Method: http.MethodGet, Host: "www.example.com", Path: "/test"},
			secondary: "",
		},
		{ // when there's Vary header field
			store: &Store{},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
				Header: http.Header{
					"Accept":          []string{"application/json"},
					"Accept-Language": []string{"ja-JP"},
				},
			},
			rep: &Representation{
				Body: []byte{},
				HeaderMap: http.Header{
					"Vary": []string{"Accept", "Accept-Language"},
				},
			},

			primary:   ResourceKey{Method: http.MethodGet, Host: "www.example.com", Path: "/test"},
			secondary: "Accept=application%2Fjson&Accept-Language=ja-JP",
		},
	}

	for _, tc := range testCases {
		tc.store.Set(tc.req, tc.rep)

		pe, ok := tc.store.Resources[tc.primary]
		if !ok {
			t.Errorf("expected store to have an entry for %#v but not", tc.primary)
		}

		rep, ok := pe.Representations[tc.secondary]
		if !ok {
			t.Errorf("expected %#v to have a reponse for %#v but not", pe, tc.secondary)
		}

		for k := range tc.rep.HeaderMap {
			if len(tc.rep.HeaderMap) != len(rep.HeaderMap) {
				t.Errorf("expected %d, got %d", len(tc.rep.HeaderMap), len(rep.HeaderMap))
			}
			for i := 0; i < len(tc.rep.HeaderMap[k]); i++ {
				if tc.rep.HeaderMap[k][i] != rep.HeaderMap[k][i] {
					t.Errorf("for header %s, expected %#v, got %#v", k, tc.rep.HeaderMap[k][i], rep.HeaderMap[k][i])
				}
			}
		}

		if string(tc.rep.Body) != string(rep.Body) {
			t.Errorf("expected %#v, got %#v", string(tc.rep.Body), string(rep.Body))
		}

		if tc.store.History.Len() != 1 {
			t.Errorf("expected 1, got %d", tc.store.History.Len())
		}

		k := tc.store.History.Front().Value.(key)
		if tc.primary != k.resource {
			t.Errorf("expected %v, got %v", tc.primary, k.resource)
		}
		if tc.secondary != k.representation {
			t.Errorf("expected %v, got %v", tc.primary, k.resource)
		}
	}
}
