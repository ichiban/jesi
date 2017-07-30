package request

import (
	"context"
	"net/http"

	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
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
	req := r.WithContext(context.WithValue(r.Context(), IDKey, rid))

	log.WithFields(log.Fields{
		"request":        rid,
		"method":         r.Method,
		"host":           r.Host,
		"url":            r.URL,
		"content_length": r.ContentLength,
		"remote_addr":    r.RemoteAddr,
	}).Info("Identified a request")

	for k, vs := range r.Header {
		for _, v := range vs {
			log.WithFields(log.Fields{
				"request": rid,
				"field":   k,
				"value":   v,
			}).Info("Found a request header")
		}
	}

	return req
}

// ID returns a unique ID of the request.
func ID(r *http.Request) string {
	v := r.Context().Value(IDKey)
	if v == nil {
		return ""
	}
	return v.(string)
}
