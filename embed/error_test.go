package embed

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestError_Error(t *testing.T) {
	testCases := []struct {
		err  *Error
		json string
	}{
		{
			err: &Error{
				Type:  "https://www.example.com/failed",
				Title: "something went wrong",
			},
			json: `{"type":"https://www.example.com/failed","title":"something went wrong"}`,
		},
		{
			err: &Error{
				Type:   "https://ichiban.github.io/jesi/problems/response-error",
				Title:  "Response Error",
				Status: http.StatusNotFound,
				Detail: http.StatusText(http.StatusNotFound),
				Links: map[string]interface{}{
					"about": map[string]interface{}{
						"href": "/foo",
					},
				},
			},
			json: `{"type":"https://ichiban.github.io/jesi/problems/response-error","title":"Response Error","status":404,"detail":"Not Found","_links":{"about":{"href":"/foo"}}}`,
		},
	}

	for _, tc := range testCases {
		b, err := json.Marshal(tc.err)
		if err != nil {
			t.Error(err)
		}
		if tc.json != string(b) {
			t.Errorf("expected: %s, got: %s", tc.json, string(b))
		}
	}
}
