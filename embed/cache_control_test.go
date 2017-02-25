package embed

import (
	"github.com/ichiban/jesi/common"
	"net/http"
	"testing"
	"time"
)

func TestNewCacheControl(t *testing.T) {
	testCases := []struct {
		resp *common.ResponseBuffer
		cc   *CacheControl
	}{
		{
			resp: &common.ResponseBuffer{
				HeaderMap: http.Header{},
			},
			cc: &CacheControl{},
		},
		{ // If we find Expires, convert it to Cache-Control: max-date.
			resp: &common.ResponseBuffer{
				HeaderMap: http.Header{
					"Date":    []string{"Thu, 01 Dec 1994 16:00:00 GMT"},
					"Expires": []string{"Thu, 01 Dec 1994 16:00:10 GMT"},
				},
			},
			cc: &CacheControl{
				MaxAge: func() *time.Duration {
					d := 10 * time.Second
					return &d
				}(),
			},
		},
		{ // If we can't parse Expires (especially "0"), that means max-age=0.
			resp: &common.ResponseBuffer{
				HeaderMap: http.Header{
					"Expires": []string{"0"},
				},
			},
			cc: &CacheControl{
				MaxAge: func() *time.Duration {
					d := time.Duration(0)
					return &d
				}(),
			},
		},
		{
			resp: &common.ResponseBuffer{
				HeaderMap: http.Header{
					"Cache-Control": []string{
						"must-revalidate",
						"no-cache",
						"no-store",
						"public",
						"private",
						"immutable",
						"max-age=123456789",
					},
				},
			},
			cc: &CacheControl{
				MustRevalidate: true,
				NoCache:        true,
				NoStore:        true,
				Public:         true,
				Private:        true,
				MaxAge: func() *time.Duration {
					d := 123456789 * time.Second
					return &d
				}(),
				Immutable: true,
			},
		},
		{
			resp: &common.ResponseBuffer{
				HeaderMap: http.Header{
					"Cache-Control": []string{"must-revalidate, no-cache, no-store, public, private, immutable, max-age=123456789"},
				},
			},
			cc: &CacheControl{
				MustRevalidate: true,
				NoCache:        true,
				NoStore:        true,
				Public:         true,
				Private:        true,
				Immutable:      true,
				MaxAge: func() *time.Duration {
					d := 123456789 * time.Second
					return &d
				}(),
			},
		},
	}

	for i, tc := range testCases {
		result := NewCacheControl(tc.resp)

		if tc.cc.MustRevalidate != result.MustRevalidate {
			t.Errorf("(%d) MustRevalidate: expected %#v, got %#v", i, tc.cc.MustRevalidate, result.MustRevalidate)
		}

		if tc.cc.NoCache != result.NoCache {
			t.Errorf("(%d) NoCache: expected %#v, got %#v", i, tc.cc.NoCache, result.NoCache)
		}

		if tc.cc.NoStore != result.NoStore {
			t.Errorf("(%d) NoStore: expected %#v, got %#v", i, tc.cc.NoStore, result.NoStore)
		}

		if tc.cc.Public != result.Public {
			t.Errorf("(%d) Public: expected %#v, got %#v", i, tc.cc.Public, result.Public)
		}

		if tc.cc.Private != result.Private {
			t.Errorf("(%d) Private: expected %#v, got %#v", i, tc.cc.Private, result.Private)
		}

		if tc.cc.Immutable != result.Immutable {
			t.Errorf("(%d) Immutable: expected %#v, got %#v", i, tc.cc.Immutable, result.Immutable)
		}

		if tc.cc.MaxAge != nil && result.MaxAge != nil {
			if *tc.cc.MaxAge != *result.MaxAge {
				t.Errorf("(%d) MaxAge: expected %#v, got %#v", i, *tc.cc.MaxAge, *result.MaxAge)
			}
		} else {
			if tc.cc.MaxAge != result.MaxAge {
				t.Errorf("(%d) MaxAge: expected %#v, got %#v", i, tc.cc.MaxAge, result.MaxAge)
			}
		}
	}
}

func TestCacheControl_String(t *testing.T) {
	testCases := []struct {
		cc  *CacheControl
		str string
	}{
		{
			cc:  &CacheControl{},
			str: "",
		},
		{
			cc: &CacheControl{
				MustRevalidate: true,
				NoCache:        true,
				NoStore:        true,
				Public:         true,
				Private:        true,
				Immutable:      true,
				MaxAge: func() *time.Duration {
					d := 123456789 * time.Second
					return &d
				}(),
			},
			str: "must-revalidate,no-cache,no-store,public,private,immutable,max-age=123456789",
		},
	}

	for i, tc := range testCases {
		result := tc.cc.String()

		if tc.str != result {
			t.Errorf("(%d) expected %#v, got %#v", i, tc.str, result)
		}
	}
}

func TestCacheControl_Merge(t *testing.T) {
	testCases := []struct {
		a      *CacheControl
		b      *CacheControl
		merged *CacheControl
	}{
		{
			a:      &CacheControl{},
			b:      &CacheControl{},
			merged: &CacheControl{},
		},
		{
			a: &CacheControl{
				MustRevalidate: true,
				NoCache:        true,
				NoStore:        true,
				Public:         true,
				Private:        true,
				Immutable:      true,
				MaxAge: func() *time.Duration {
					d := 123456789 * time.Second
					return &d
				}(),
			},
			b: &CacheControl{},
			merged: &CacheControl{
				MustRevalidate: true,
				NoCache:        true,
				NoStore:        true,
				Public:         false,
				Private:        true,
				Immutable:      false,
				MaxAge: func() *time.Duration {
					d := 123456789 * time.Second
					return &d
				}(),
			},
		},
		{ // Smaller MaxAge proceeds.
			a: &CacheControl{
				MaxAge: func() *time.Duration {
					d := 1 * time.Second
					return &d
				}(),
			},
			b: &CacheControl{
				MaxAge: func() *time.Duration {
					d := 2 * time.Second
					return &d
				}(),
			},
			merged: &CacheControl{
				MaxAge: func() *time.Duration {
					d := 1 * time.Second
					return &d
				}(),
			},
		},
	}

	for i, tc := range testCases {
		result := tc.a.Merge(tc.b)

		if tc.merged.MustRevalidate != result.MustRevalidate {
			t.Errorf("(%d) MustRevalidate: expected %#v, got %#v", i, tc.merged.MustRevalidate, result.MustRevalidate)
		}

		if tc.merged.NoCache != result.NoCache {
			t.Errorf("(%d) NoCache: expected %#v, got %#v", i, tc.merged.NoCache, result.NoCache)
		}

		if tc.merged.NoStore != result.NoStore {
			t.Errorf("(%d) NoStore: expected %#v, got %#v", i, tc.merged.NoStore, result.NoStore)
		}

		if tc.merged.Public != result.Public {
			t.Errorf("(%d) Public: expected %#v, got %#v", i, tc.merged.Public, result.Public)
		}

		if tc.merged.Private != result.Private {
			t.Errorf("(%d) Private: expected %#v, got %#v", i, tc.merged.Private, result.Private)
		}

		if tc.merged.Immutable != result.Immutable {
			t.Errorf("(%d) Immutable: expected %#v, got %#v", i, tc.merged.Immutable, result.Immutable)
		}

		if tc.merged.MaxAge != nil && result.MaxAge != nil {
			if *tc.merged.MaxAge != *result.MaxAge {
				t.Errorf("(%d) MaxAge: expected %#v, got %#v", i, *tc.merged.MaxAge, *result.MaxAge)
			}
		} else {
			if tc.merged.MaxAge != result.MaxAge {
				t.Errorf("(%d) MaxAge: expected %#v, got %#v", i, tc.merged.MaxAge, result.MaxAge)
			}
		}
	}
}
