package embed

import (
	"fmt"
	"net/http"
)

// Error represents an embedding error.
type Error struct {
	Type   string                 `json:"type"`
	Title  string                 `json:"title"`
	Status int                    `json:"status,omitempty"`
	Detail string                 `json:"detail,omitempty"`
	Links  map[string]interface{} `json:"_links,omitempty"`
}

var _ error = (*Error)(nil)

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Title, e.Detail)
}

// NewMalformedURLError returns an error for an invalid link URI.
func NewMalformedURLError(err error) *Error {
	return &Error{
		Type:   "https://ichiban.github.io/jesi/problems/malformed-url",
		Title:  "Malformed URL",
		Detail: err.Error(),
	}
}

// NewMalformedSubRequestError returns an error for an invalid sub request.
func NewMalformedSubRequestError(err error, uri fmt.Stringer) *Error {
	return &Error{
		Type:   "https://ichiban.github.io/jesi/problems/malformed-subrequest",
		Title:  "Malformed Subrequest",
		Detail: err.Error(),
		Links: map[string]interface{}{
			about: uri.String(),
		},
	}
}

// NewRoundTripError returns an error for a failed sub request round trip.
func NewRoundTripError(err error, uri fmt.Stringer) *Error {
	return &Error{
		Type:   "https://ichiban.github.io/jesi/problems/round-trip-error",
		Title:  "Round Trip Error",
		Detail: err.Error(),
		Links: map[string]interface{}{
			about: uri.String(),
		},
	}
}

// NewResponseError returns an error for a non-successful sub request response.
func NewResponseError(resp *http.Response, uri fmt.Stringer) *Error {
	return &Error{
		Type:   "https://ichiban.github.io/jesi/problems/response-error",
		Title:  "Response Error",
		Status: resp.StatusCode,
		Detail: http.StatusText(resp.StatusCode),
		Links: map[string]interface{}{
			about: uri.String(),
		},
	}
}

// NewResponseBodyReadError returns an error for a failed response body read.
func NewResponseBodyReadError(err error, uri fmt.Stringer) *Error {
	return &Error{
		Type:   "https://ichiban.github.io/jesi/problems/response-body-read-error",
		Title:  "Response Body Read Error",
		Detail: err.Error(),
		Links: map[string]interface{}{
			about: uri.String(),
		},
	}
}

// NewMalformedJSONError returns an error for an invalid response JSON.
func NewMalformedJSONError(err error, uri fmt.Stringer) *Error {
	return &Error{
		Type:   "https://ichiban.github.io/jesi/problems/malformed-json",
		Title:  "Malformed JSON",
		Detail: err.Error(),
		Links: map[string]interface{}{
			about: uri.String(),
		},
	}
}
