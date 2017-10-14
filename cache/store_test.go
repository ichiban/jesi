package cache

import (
	"container/list"
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
					ResourceKey{Host: "www.example.com", Path: "/test"}: {
						Representations: map[RepresentationKey]*Representation{
							{Method: http.MethodGet, Key: ""}: {
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
					ResourceKey{Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						Representations: map[RepresentationKey]*Representation{
							{Method: http.MethodGet, Key: "Accept=application%2Fjson&Accept-Language=ja-JP"}: {
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
					ResourceKey{Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						Representations: map[RepresentationKey]*Representation{
							{Method: http.MethodGet, Key: "Accept=application%2Fjson&Accept-Language=ja-JP"}: {
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
					ResourceKey{Host: "www.example.com", Path: "/test"}: {
						Representations: map[RepresentationKey]*Representation{
							{Method: http.MethodGet, Key: ""}: {
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

			primary:   ResourceKey{Host: "www.example.com", Path: "/test"},
			secondary: RepresentationKey{Method: http.MethodGet, Key: ""},
		},
		{ // when there's an existing entry for the request (replace)
			store: &Store{
				Resources: map[ResourceKey]*Resource{
					ResourceKey{Host: "www.example.com", Path: "/test"}: {
						Representations: map[RepresentationKey]*Representation{
							{Method: http.MethodGet, Key: ""}: {},
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

			primary:   ResourceKey{Host: "www.example.com", Path: "/test"},
			secondary: RepresentationKey{Method: http.MethodGet, Key: ""},
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

			primary:   ResourceKey{Host: "www.example.com", Path: "/test"},
			secondary: RepresentationKey{Method: http.MethodGet, Key: "Accept=application%2Fjson&Accept-Language=ja-JP"},
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

func TestStore_Purge(t *testing.T) {
	reps := []*Representation{
		{
			Body: []byte(`{"test":"ok"}`),
		},
		{
			Body: []byte(`{"foo":"bar"}`),
		},
	}

	keys := []key{
		{
			resource: ResourceKey{
				Host: "www.example.com",
				Path: "/test",
			},
			representation: RepresentationKey{
				Method: http.MethodGet,
				Key:    "",
			},
		},
		{
			resource: ResourceKey{
				Host: "www.example.com",
				Path: "/foo",
			},
			representation: RepresentationKey{
				Method: http.MethodGet,
				Key:    "",
			},
		},
	}

	history := func(reps []*Representation, keys []key) *list.List {
		l := list.New()
		for i, rep := range reps {
			rep.Element = l.PushBack(keys[i])
		}
		return l
	}

	testCases := []struct {
		before *Store
		req    *http.Request

		after *Store
	}{
		{ // when it's not cached
			before: &Store{},
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/test",
				},
			},

			after: &Store{
				History: history(nil, nil),
			},
		},
		{ // when it's cached
			before: &Store{
				Resources: map[ResourceKey]*Resource{
					ResourceKey{Host: "www.example.com", Path: "/test"}: {
						Representations: map[RepresentationKey]*Representation{
							{Method: http.MethodGet, Key: ""}: reps[0],
						},
					},
					ResourceKey{Host: "www.example.com", Path: "/foo"}: {
						Representations: map[RepresentationKey]*Representation{
							{Method: http.MethodGet, Key: ""}: reps[1],
						},
					},
				},
				History: history([]*Representation{
					reps[0],
					reps[1],
				}, []key{
					keys[0],
					keys[1],
				}),
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/test",
				},
			},

			after: &Store{
				Resources: map[ResourceKey]*Resource{
					ResourceKey{Host: "www.example.com", Path: "/foo"}: {
						Representations: map[RepresentationKey]*Representation{
							{Method: http.MethodGet, Key: ""}: reps[1],
						},
					},
				},
				History: history([]*Representation{
					reps[1],
				}, []key{
					keys[1],
				}),
			},
		},
	}

	for i, tc := range testCases {
		store := tc.before
		store.Purge(tc.req)

		if len(tc.after.Resources) != len(store.Resources) {
			t.Errorf("(%d) [len(Resources)] expected: %d, got: %d", i, len(tc.after.Resources), len(store.Resources))
		}

		for resKey, res := range tc.after.Resources {
			if len(res.Fields) != len(store.Resources[resKey].Fields) {
				t.Errorf("(%d) [len(Resources[%s])] expected: %d, got: %d", i, resKey, len(res.Fields), len(store.Resources[resKey].Fields))
				continue
			}

			for j, f := range res.Fields {
				if f != store.Resources[resKey].Fields[j] {
					t.Errorf("(%d, %d) expected: %s, got: %s", i, j, f, store.Resources[resKey].Fields[j])
				}
			}

			for repKey, rep := range res.Representations {
				if rep.StatusCode != store.Resources[resKey].Representations[repKey].StatusCode {
					t.Errorf("(%d) expected: %d, got: %d", i, rep.StatusCode, store.Resources[resKey].Representations[repKey].StatusCode)
				}

				if string(rep.Body) != string(store.Resources[resKey].Representations[repKey].Body) {
					t.Errorf("(%d) expected: %s, got: %s", i, string(rep.Body), string(store.Resources[resKey].Representations[repKey].Body))
				}
			}
		}

		if tc.after.History.Len() != store.History.Len() {
			t.Errorf("(%d) expected: %d, got: %d", i, tc.after.History.Len(), store.History.Len())
		}

		e := tc.after.History.Front()
		g := store.History.Front()
		for e != nil && g != nil {
			if _, ok := e.Value.(key); !ok {
				t.Errorf("(%d) [e] expected key, got: %#v", i, e.Value)
			}

			if _, ok := g.Value.(key); !ok {
				t.Errorf("(%d) [g] expected key, got: %#v", i, g.Value)
			}

			e = e.Next()
			g = g.Next()
		}
	}
}
