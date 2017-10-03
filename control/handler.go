package control

import (
	"net/http"
	"regexp"
	"sync"

	"github.com/ichiban/jesi/transaction"
	log "github.com/sirupsen/logrus"
)

const (
	authorization = "Authorization"
)

var (
	authorizationPattern = regexp.MustCompile(`\A\s*(?i:bearer)\s+([[:alnum:]-._~+/]+)\s*\z`)
)

// Handler handles control requests with bearer token.
type Handler struct {
	*LogStream

	Secret string
	Next   http.Handler
}

var _ http.Handler = (*Handler)(nil)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Secret == "" {
		log.WithFields(log.Fields{
			"id": transaction.ID(r),
		}).Debug("secret is not given")

		h.Next.ServeHTTP(w, r)
		return
	}

	m := authorizationPattern.FindStringSubmatch(r.Header.Get(authorization))
	if len(m) != 2 {
		log.WithFields(log.Fields{
			"id": transaction.ID(r),
		}).Debug("Authorization header doesn't match")

		h.Next.ServeHTTP(w, r)
		return
	}
	if h.Secret != m[1] {
		log.WithFields(log.Fields{
			"id": transaction.ID(r),
		}).Debug("secret doesn't match")

		h.Next.ServeHTTP(w, r)
		return
	}

	switch r.Method {
	case "PURGE":
		// TODO: purge cache
		w.WriteHeader(http.StatusNotFound)
	case http.MethodGet:
		switch r.URL.Path {
		case "/logs":
			h.handleLogs(w, r)
		case "/metrics":
			// TODO: prometheus metrics
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	ch := make(chan []byte)
	h.Tap(ch)
	defer h.Untap(ch)

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	for {
		select {
		case b := <-ch:
			if _, err := w.Write(b); err != nil {
				log.WithFields(log.Fields{
					"error": err,
				}).Error("failed to write a log entry")
				continue
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

// LogStream receives log entries and duplicate them to taps.
type LogStream struct {
	sync.RWMutex
	log.JSONFormatter

	Taps  map[chan []byte]struct{}
	OnTap chan struct{}
}

var _ log.Hook = (*LogStream)(nil)

// Levels are the log levels handled by LogStream.
func (s *LogStream) Levels() []log.Level {
	return []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
		log.InfoLevel,
	}
}

// Fire serializes a log entry and spread them to taps.
func (s *LogStream) Fire(e *log.Entry) error {
	s.RLock()
	defer s.RUnlock()

	b, err := s.Format(e)
	if err != nil {
		return err
	}

	for ch := range s.Taps {
		ch <- b
	}

	return nil
}

// Tap registers a chan to the LogStream to receive serialized log entries.
func (s *LogStream) Tap(ch chan []byte) {
	s.Lock()
	defer s.Unlock()

	if s.Taps == nil {
		s.Taps = make(map[chan []byte]struct{})
	}

	s.Taps[ch] = struct{}{}

	if s.OnTap != nil {
		s.OnTap <- struct{}{}
	}
}

// Untap unregisters a chan from the LogStream.
func (s *LogStream) Untap(ch chan []byte) {
	s.Lock()
	defer s.Unlock()

	delete(s.Taps, ch)
}
