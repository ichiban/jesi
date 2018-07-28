package transaction

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Key is an identifier of a context value associated with requests.
type Key int

const (
	// IDKey is a key for request IDs.
	IDKey Key = iota
)

var genID = func() uuid.UUID {
	return uuid.New()
}

// WithID returns a request with unique ID based on a given request.
func WithID(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), IDKey, genID()))
}

// ID returns a unique ID of the request.
func ID(r *http.Request) *uuid.UUID {
	v := r.Context().Value(IDKey)
	if v == nil {
		return nil
	}
	id := v.(uuid.UUID)
	return &id
}
