package balance

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"sync"
)

// Handler is a reverse proxy with multiple backends.
type Handler struct {
	httputil.ReverseProxy

	Backends *Backends
}

var _ http.Handler = (*Handler)(nil)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Director == nil {
		h.Director = h.direct
	}

	h.ReverseProxy.ServeHTTP(w, r)
}

func (h *Handler) direct(r *http.Request) {
	b := h.Backends.Next()

	if b == nil {
		// TODO: cancel r
		return
	}

	log.Printf("balance: %s", b.URL)

	r.URL.Scheme = b.URL.Scheme
	r.URL.Host = b.URL.Host
	r.URL.Path = path.Join(b.URL.Path, r.URL.Path)
	if b.URL.RawQuery == "" || r.URL.RawQuery == "" {
		r.URL.RawQuery = b.URL.RawQuery + r.URL.RawQuery
	} else {
		r.URL.RawQuery = b.URL.RawQuery + "&" + r.URL.RawQuery
	}
	if _, ok := r.Header["User-Agent"]; !ok {
		// explicitly disable User-Agent so it's not set to default value
		r.Header.Set("User-Agent", "")
	}
}

// Backend represents an upstream server.
type Backend struct {
	URL *url.URL
}

func (b *Backend) String() string {
	return b.URL.String()
}

// Backends hold a set of backends.
type Backends struct {
	sync.RWMutex

	Pos int
	All []Backend
}

var _ flag.Value = (*Backends)(nil)

func (b *Backends) String() string {
	b.RLock()
	defer b.RUnlock()

	var res []string
	for _, b := range b.All {
		res = append(res, b.String())
	}

	return strings.Join(res, ", ")
}

// Set adds a new backend represented by the given URL string.
func (b *Backends) Set(str string) error {
	uri, err := url.Parse(str)
	if err != nil {
		return err
	}

	b.Lock()
	defer b.Unlock()

	b.All = append(b.All, Backend{
		URL: uri,
	})

	return nil
}

// Next picks one of the backends and returns.
func (b *Backends) Next() *Backend {
	b.Lock()
	defer b.Unlock()

	if len(b.All) == 0 {
		return nil
	}

	res := b.All[b.Pos]
	b.Pos = (b.Pos + 1) % len(b.All)
	return &res
}
