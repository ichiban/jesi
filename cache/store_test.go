package cache

import (
	"net/http"
	"net/url"
	"testing"
	"time"
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
					{Host: "www.example.com", Path: "/test"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: ""}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body: []byte(`{"foo":"bar"}`),
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
					{Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: "Accept=application%2Fjson&Accept-Language=ja-JP"}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: "Accept=application%2Fjson&Accept-Language=ja-JP"}}: {
						Body: []byte(`{"foo":"bar"}`),
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
					{Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: "Accept=application%2Fjson&Accept-Language=ja-JP"}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: "Accept=application%2Fjson&Accept-Language=ja-JP"}}: {
						Body: []byte(`{"foo":"bar"}`),
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
					{Host: "www.example.com", Path: "/test"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: ""}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body: []byte(`{"foo":"bar"}`),
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
		before *Store
		req    *http.Request
		rep    *Representation

		after *Store
	}{
		{ // when there's no entry for the request (insert)
			before: &Store{},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},
			rep: &Representation{
				Body: []byte(`{"foo":"bar"}`),
			},

			after: &Store{
				InUse: 13,
				Resources: map[ResourceKey]*Resource{
					{Host: "www.example.com", Path: "/test"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							RepresentationKey{Method: http.MethodGet, Key: ""}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body: []byte(`{"foo":"bar"}`),
					},
				},
			},
		},
		{ // when there's an existing entry for the request (replace)
			before: &Store{
				InUse: 13,
				Resources: map[ResourceKey]*Resource{
					{Host: "www.example.com", Path: "/test"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: ""}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body: []byte(`{"test":"ok"}`),
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

			after: &Store{
				InUse: 0,
				Resources: map[ResourceKey]*Resource{
					{Host: "www.example.com", Path: "/test"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: ""}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body: []byte(``),
					},
				},
			},
		},
		{ // when there's Vary header field
			before: &Store{},
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

			after: &Store{
				InUse: 0,
				Resources: map[ResourceKey]*Resource{
					{Host: "www.example.com", Path: "/test"}: {
						Fields: []string{"Accept", "Accept-Language"},
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: "Accept=application%2Fjson&Accept-Language=ja-JP"}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: "Accept=application%2Fjson&Accept-Language=ja-JP"}}: {
						Body: []byte{},
						HeaderMap: http.Header{
							"Vary": []string{"Accept", "Accept-Language"},
						},
					},
				},
			},
		},
		{ // when it exceeds the limit
			before: &Store{
				Max:    13,
				InUse:  12,
				Sample: 2,
				Resources: map[ResourceKey]*Resource{
					{Host: "www.example.com", Path: "/foo"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: ""}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/foo"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body:         []byte(`{"foo":"ok"}`),
						LastUsedTime: time.Now().Add(-10 * time.Minute),
					},
				},
			},
			req: &http.Request{
				Method: http.MethodGet,
				URL:    url,
			},
			rep: &Representation{
				Body: []byte(`{"test":"ok"}`),
			},

			after: &Store{
				InUse: 13,
				Resources: map[ResourceKey]*Resource{
					{Host: "www.example.com", Path: "/test"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: ""}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body: []byte(`{"test":"ok"}`),
					},
				},
			},
		},
	}

	for i, tc := range testCases {
		store := tc.before
		store.Set(tc.req, tc.rep)

		if tc.after.InUse != store.InUse {
			t.Errorf("(%d) [InUse] expected: %d, got: %d", i, tc.after.InUse, store.InUse)
		}

		if len(tc.after.Resources) != len(store.Resources) {
			t.Errorf("(%d) [len(Resources)] expected: %d, got: %d", i, len(tc.after.Resources), len(store.Resources))
		}

		for resKey, res := range tc.after.Resources {
			if len(res.Fields) != len(store.Resources[resKey].Fields) {
				t.Errorf("(%d) [len(Resources[%s].Fields)] expected: %d, got: %d", i, resKey, len(res.Fields), len(store.Resources[resKey].Fields))
				continue
			}

			for j, f := range res.Fields {
				if f != store.Resources[resKey].Fields[j] {
					t.Errorf("(%d, %d) expected: %s, got: %s", i, j, f, store.Resources[resKey].Fields[j])
				}
			}
		}

		for key, rep := range tc.after.Representations {
			if rep.StatusCode != store.Representations[key].StatusCode {
				t.Errorf("(%d) expected: %d, got: %d", i, rep.StatusCode, store.Representations[key].StatusCode)
			}

			if string(rep.Body) != string(store.Representations[key].Body) {
				t.Errorf("(%d) expected: %s, got: %s", i, string(rep.Body), string(store.Representations[key].Body))
			}
		}
	}
}

func TestStore_Purge(t *testing.T) {
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

			after: &Store{},
		},
		{ // when it's cached
			before: &Store{
				Resources: map[ResourceKey]*Resource{
					{Host: "www.example.com", Path: "/test"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: ""}: {},
						},
					},
					{Host: "www.example.com", Path: "/foo"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: ""}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/test"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body: []byte(`{"test":"ok"}`),
					},
					{ResourceKey{Host: "www.example.com", Path: "/foo"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body: []byte(`{"foo":"bar"}`),
					},
				},
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
					{Host: "www.example.com", Path: "/foo"}: {
						RepresentationKeys: map[RepresentationKey]struct{}{
							{Method: http.MethodGet, Key: ""}: {},
						},
					},
				},
				Representations: map[Key]*Representation{
					{ResourceKey{Host: "www.example.com", Path: "/foo"}, RepresentationKey{Method: http.MethodGet, Key: ""}}: {
						Body: []byte(`{"foo":"bar"}`),
					},
				},
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
		}

		for key, rep := range tc.after.Representations {
			if rep.StatusCode != store.Representations[key].StatusCode {
				t.Errorf("(%d) expected: %d, got: %d", i, rep.StatusCode, store.Representations[key].StatusCode)
			}

			if string(rep.Body) != string(store.Representations[key].Body) {
				t.Errorf("(%d) expected: %s, got: %s", i, string(rep.Body), string(store.Representations[key].Body))
			}

		}
	}
}
