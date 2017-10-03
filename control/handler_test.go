package control

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/ichiban/jesi/cache"
)

func TestHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		secret string
		req    *http.Request
		logs   []*log.Entry

		passThrough bool
		resp        *http.Response
	}{
		{ // no secret
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/logs",
				},
			},

			passThrough: true,
		},
		{ // request without secret
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/logs",
				},
			},

			passThrough: true,
		},
		{ // request with wrong secret
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/logs",
				},
				Header: http.Header{
					"Authorization": []string{"bearer bar"},
				},
			},

			passThrough: true,
		},
		{ // unknown method
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/unknown",
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
		{ // logs
			secret: "foo",
			req: &http.Request{
				Method: http.MethodGet,
				URL: &url.URL{
					Path: "/logs",
				},
				Header: http.Header{
					"Authorization": []string{"bearer foo"},
				},
			},
			logs: []*log.Entry{
				{
					Data: log.Fields{
						"foo": "123",
					},
				},
				{
					Data: log.Fields{
						"bar": "456",
					},
				},
			},

			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/x-ndjson"},
				},
				Body: ioutil.NopCloser(strings.NewReader(strings.Join([]string{
					`{"foo":"123","level":"panic","msg":"","time":"0001-01-01T00:00:00Z"}`, "\n",
					`{"bar":"456","level":"panic","msg":"","time":"0001-01-01T00:00:00Z"}`, "\n",
				}, ""))),
			},
		},
	}

	for i, tc := range testCases {
		ls := LogStream{
			OnTap: make(chan struct{}),
		}
		var h testHandler
		handler := &Handler{
			Secret:    tc.secret,
			LogStream: &ls,
			Next:      &h,
		}

		c, cancel := context.WithCancel(tc.req.Context())
		tc.req = tc.req.WithContext(c)
		go func() {
			<-ls.OnTap
			for _, e := range tc.logs {
				ls.Fire(e)
			}
			cancel()
		}()

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
			t.Errorf("(%d) [Body] expected: %s, got: %s", i, string(b), string(resp.Body))
		}
	}
}

func TestLogStream_Levels(t *testing.T) {
	tc := []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
		log.InfoLevel,
	}

	var ls LogStream
	for i, l := range ls.Levels() {
		if tc[i] != l {
			t.Errorf("(%d) expected: %s, got %s", i, tc[i], l)
		}
	}
}

func TestLogStream_Fire(t *testing.T) {
	testCases := []struct {
		e log.Entry

		b   []byte
		err error
	}{
		{
			e: log.Entry{},

			b: []byte(strings.Join([]string{
				`{"level":"panic","msg":"","time":"0001-01-01T00:00:00Z"}`, "\n",
			}, "")),
		},
	}

	var ls LogStream

	for i, tc := range testCases {
		ch := make(chan []byte, 1)
		ls.Tap(ch)
		err := ls.Fire(&tc.e)

		if tc.err != err {
			t.Errorf("(%d) expected: %v, got: %v", i, tc.err, err)
		}

		b := <-ch

		if string(tc.b) != string(b) {
			t.Errorf("(%d) expected: %s, got: %s", i, string(tc.b), string(b))
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
