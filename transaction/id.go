package transaction

import (
	"context"
	"net/http"

	"github.com/satori/go.uuid"
)

// Key is an identifier of a context value associated with requests.
type Key int

const (
	// IDKey is a key for request IDs.
	IDKey Key = iota
)

var genID = func() string {
	return uuid.NewV4().String()
}

// WithID returns a request with unique ID based on a given request.
func WithID(r *http.Request) *http.Request {
	rid := genID()
	return r.WithContext(context.WithValue(r.Context(), IDKey, rid))
}

// ID returns a unique ID of the request.
func ID(r *http.Request) string {
	v := r.Context().Value(IDKey)
	if v == nil {
		return ""
	}
	return v.(string)
}
