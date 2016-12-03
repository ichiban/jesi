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
				Title: "something went wrong",
			},
			json: `{"title":"something went wrong"}`,
		},
		{
			err: &Error{
				Status: http.StatusNotFound,
				Title:  "Error Response",
				Detail: http.StatusText(http.StatusNotFound),
				Links: map[string]interface{}{
					"about": map[string]interface{}{
						"href": "/foo",
					},
				},
			},
			json: `{"status":404,"title":"Error Response","detail":"Not Found","_links":{"about":{"href":"/foo"}}}`,
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
