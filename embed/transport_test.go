package embed

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

func TestTransport_RoundTrip(t *testing.T) {
	testCases := []struct {
		url       string
		resources map[string]string
		calls     []string
		body      string
	}{
		{ // without 'with' query parameter, it simply returns JSON.
			url: "/test",
			resources: map[string]string{
				"/test": `{}`,
			},
			calls: []string{"/test"},
			body:  `{}`,
		},
		{ // with 'with' query parameter, it embeds resources specified by edges.
			url: "/pen?with=next.next.next",
			resources: map[string]string{
				"/pen":       `{"_links":{"next":{"href":"/pineapple"},"self":{"href":"/pen"}}}`,
				"/pineapple": `{"_links":{"next":{"href":"/apple"},"self":{"href":"/pineapple"}}}`,
				"/apple":     `{"_links":{"next":{"href":"/pen"},"self":{"href":"/apple"}}}`,
			},
			calls: []string{"/pen", "/pineapple", "/apple", "/pen"},
			body:  `{"_embedded":{"next":{"_embedded":{"next":{"_embedded":{"next":{"_links":{"next":{"href":"/pineapple"},"self":{"href":"/pen"}}}},"_links":{"next":{"href":"/pen"},"self":{"href":"/apple"}}}},"_links":{"next":{"href":"/apple"},"self":{"href":"/pineapple"}}}},"_links":{"next":{"href":"/pineapple"},"self":{"href":"/pen"}}}`,
		},
		{ // if the specified edge is not found, it embeds a corresponding error document JSON.
			url: "/foo?with=bar",
			resources: map[string]string{
				"/foo": `{"_links":{"bar":{"href":"/bar"},"self":{"href":"/foo"}}}`,
			},
			calls: []string{"/foo"},
			body:  `{"_embedded":{"errors":[{"status":404,"title":"Error Response","detail":"Not Found","_links":{"about":"/bar"}}]},"_links":{"bar":{"href":"/bar"},"self":{"href":"/foo"}}}`,
		},
	}

	for _, tc := range testCases {
		req, err := http.NewRequest(http.MethodGet, tc.url, bytes.NewReader([]byte{}))
		if err != nil {
			t.Errorf("err is not nil: %v", err)
		}

		tt := &testTransport{
			T:         t,
			Resources: tc.resources,
		}
		e := Transport{tt}

		r, err := e.RoundTrip(req)
		if err != nil {
			t.Errorf("err is not nil: %v", err)
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Errorf("err is not nil: %v", err)
		}

		if tc.body != string(body) {
			t.Errorf("expected: %s, got: %s", tc.body, string(body))
		}

		tt.assert(tc.calls)
	}
}

type testTransport struct {
	T         *testing.T
	Resources map[string]string
	Actual    []string
}

var _ http.RoundTripper = (*testTransport)(nil)

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		t.T.Errorf("method is not GET: %s", req.Method)
	}

	body, ok := t.Resources[req.URL.String()]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       ioutil.NopCloser(strings.NewReader("")),
		}
		return resp, nil
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       ioutil.NopCloser(strings.NewReader(body)),
	}

	t.Actual = append(t.Actual, req.URL.String())

	return resp, nil
}

func (t *testTransport) assert(expectations []string) {
	if len(expectations) != len(t.Actual) {
		t.T.Errorf("%d expected, got: %d", len(expectations), len(t.Actual))
	}

	for i := range t.Actual {
		if expectations[i] != t.Actual[i] {
			t.T.Errorf("expected %s, got: %s", expectations[i], t.Actual[i])
		}
	}
}
