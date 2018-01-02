package control

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ichiban/jesi/cache"
	"github.com/satori/go.uuid"
)

func TestHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		secret string
		req    *http.Request

		passThrough bool
		resp        *http.Response
	}{
		{ // no secret
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/_jesi/resources",
				},
			},

			passThrough: true,
		},
		{ // request without secret
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/_jesi/resources",
				},
			},

			passThrough: true,
		},
		{ // request with wrong secret
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/_jesi/resources",
				},
				Header: http.Header{
					"Authorization": []string{"bearer bar"},
				},
			},

			passThrough: true,
		},
		{ // unknown endpoint
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/_jesi/unknown",
				},
				Header: http.Header{
					"Authorization": []string{"bearer foo"},
				},
			},

			resp: &http.Response{
				StatusCode: http.StatusNotFound,
				Header: http.Header{
					"Content-Type":           []string{"text/plain; charset=utf-8"},
					"X-Content-Type-Options": []string{"nosniff"},
				},
				Body: ioutil.NopCloser(strings.NewReader("404 page not found\n")),
			},
		},
		{ // resources
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/_jesi/resources",
				},
				Header: http.Header{
					"Authorization": []string{"bearer foo"},
				},
			},

			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": {"application/hal+json"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"_links":{"self":{"href":"//www.example.com/_jesi/resources"},"elements":[{"href":"//www.example.com/_jesi/resources/foo"}]}}`)),
			},
		},
		{ // resource
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/_jesi/resources/foo",
				},
				Header: http.Header{
					"Authorization": []string{"bearer foo"},
				},
			},

			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": {"application/hal+json"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"_links":{"self":{"href":"//www.example.com/_jesi/resources/foo"},"about":{"href":"//www.example.com/foo"},"reps":[{"href":"//www.example.com/_jesi/reps/%25s/026a1a9d-4744-4812-b4c7-0b83feda7244"}]},"unique":false,"fields":null}`)),
			},
		},
		{ // representation
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/_jesi/reps/026a1a9d-4744-4812-b4c7-0b83feda7244",
				},
				Header: http.Header{
					"Authorization": []string{"bearer foo"},
				},
			},

			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": {"application/hal+json"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"_links":{"self":{"href":"//www.example.com/_jesi/reps/%25s/026a1a9d-4744-4812-b4c7-0b83feda7244"},"resource":{"href":"//www.example.com/_jesi/resources/foo"}},"status":0,"header":null,"contentLength":0,"requestTime":"0001-01-01T00:00:00Z","responseTime":"0001-01-01T00:00:00Z","lastUsedTime":"0001-01-01T00:00:00Z"}`)),
			},
		},
		{ // purge success
			secret: "foo",
			req: &http.Request{
				Method: "PURGE",
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/foo",
				},
				Header: http.Header{
					"Authorization": []string{"bearer foo"},
				},
			},

			resp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader(`{"_links":{"self":{"href":"//www.example.com/_jesi/resources/foo"},"about":{"href":"//www.example.com/foo"}},"_embed":{"reps":[{"_links":{"self":{"href":"//www.example.com/_jesi/reps/%25s/026a1a9d-4744-4812-b4c7-0b83feda7244"},"resource":{"href":"//www.example.com/_jesi/resources/foo"}},"status":0,"header":null,"contentLength":0,"requestTime":"0001-01-01T00:00:00Z","responseTime":"0001-01-01T00:00:00Z","lastUsedTime":"0001-01-01T00:00:00Z"}]},"unique":false,"fields":null}`)),
			},
		},
		{ // purge failure
			secret: "foo",
			req: &http.Request{
				Method: "PURGE",
				URL: &url.URL{
					Host: "www.example.com",
					Path: "/bar",
				},
				Header: http.Header{
					"Authorization": []string{"bearer foo"},
				},
			},

			resp: &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			},
		},
	}

	id, err := uuid.FromString("026a1a9d-4744-4812-b4c7-0b83feda7244")
	if err != nil {
		t.Fatal(err)
	}

	for i, tc := range testCases {
		var h testHandler
		handler := &Handler{
			Store: &cache.Store{
				Resources: map[cache.ResourceKey]*cache.Resource{
					{Host: "www.example.com", Path: "/foo"}: {
						ResourceKey: cache.ResourceKey{
							Host: "www.example.com",
							Path: "/foo",
						},
						Representations: map[cache.RepresentationKey]*cache.Representation{
							{Method: http.MethodGet}: {
								ResourceKey: cache.ResourceKey{
									Host: "www.example.com",
									Path: "/foo",
								},
								ID: id,
							},
						},
					},
				},
				Representations: map[uuid.UUID]*cache.Representation{
					id: {
						ResourceKey: cache.ResourceKey{
							Host: "www.example.com",
							Path: "/foo",
						},
						ID: id,
					},
				},
			},
			Secret: tc.secret,
			Next:   &h,
		}

		var resp cache.Representation
		handler.ServeHTTP(&resp, tc.req)

		if tc.passThrough {
			if !h.Called {
				t.Errorf("(%d) [passThrough] expected: to pass through, got: cnotrol response", i)
			}
			continue
		}

		if tc.resp.StatusCode != resp.StatusCode {
			t.Errorf("(%d) [StatusCode] expected: %d, got: %d", i, tc.resp.StatusCode, resp.StatusCode)
		}

		if len(tc.resp.Header) != len(resp.HeaderMap) {
			t.Errorf("(%d) [len(Header)] expected: %v, got: %v", i, tc.resp.Header, resp.HeaderMap)
		}

		for k, vs := range tc.resp.Header {
			for j, v := range vs {
				if v != resp.HeaderMap[k][j] {
					t.Errorf("(%d, %d) [Header(%s)] expected: %s, got: %s", i, j, k, v, resp.HeaderMap[k][j])
				}
			}
		}

		b, err := ioutil.ReadAll(tc.resp.Body)
		if err != nil {
			t.Errorf("(%d) [ReadAll] expected: nil, got: %v", i, err)
		}

		if string(b) != string(resp.Body) {
			t.Errorf("(%d) [Body] expected: %v, got: %v", i, string(b), string(resp.Body))
		}
	}
}

type testHandler struct {
	Called bool
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Called = true
	w.WriteHeader(http.StatusOK)
}
