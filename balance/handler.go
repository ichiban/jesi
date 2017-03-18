package balance

import (
	"container/list"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// Handler is a reverse proxy with multiple backends.
type Handler struct {
	httputil.ReverseProxy
	*BackendPool
}

var _ http.Handler = (*Handler)(nil)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Director == nil {
		h.Director = h.direct
	}

	h.ReverseProxy.ServeHTTP(w, r)
}

func (h *Handler) direct(r *http.Request) {
	b := h.BackendPool.Next()

	if b == nil {
		return
	}

	log.Printf("balance from: %s", h.BackendPool)
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
	*list.Element
	*url.URL
	http.Client

	Sick     bool
	Interval time.Duration
}

func (b *Backend) String() string {
	return b.URL.String()
}

// Run keeps probing the backend to keep its state updated.
// When state changed, it notifies ch.
func (b *Backend) Run(ch chan<- *Backend, quit <-chan struct{}) {
	if b.Interval == 0 {
		b.Interval = 10 * time.Second
	}

	b.Client.Timeout = 10 * time.Second

	for {
		select {
		case <-time.After(b.Interval):
			old := b.Sick
			b.Probe()
			if old == b.Sick {
				continue
			}
			ch <- b
		case <-quit:
			return
		}
	}
}

// Probe makes a probing request to the background and changes its internal state accordingly.
func (b *Backend) Probe() {
	resp, err := b.Get(b.URL.String())
	if err != nil {
		b.Sick = true
		return
	}

	b.Sick = resp.StatusCode >= 400
}

// BackendPool hold a set of backends.
type BackendPool struct {
	sync.RWMutex

	Changed chan *Backend
	Healthy list.List
	Sick    list.List
}

var _ flag.Value = (*BackendPool)(nil)

func (p *BackendPool) String() string {
	p.RLock()
	defer p.RUnlock()

	var h []string
	for e := p.Healthy.Front(); e != nil; e = e.Next() {
		h = append(h, e.Value.(*Backend).String())
	}

	var s []string
	for e := p.Sick.Front(); e != nil; e = e.Next() {
		s = append(s, e.Value.(*Backend).String())
	}

	return fmt.Sprintf("healthy: [%s], sick: [%s]", strings.Join(h, ", "), strings.Join(s, ", "))
}

// Set adds a new backend represented by the given URL string.
func (p *BackendPool) Set(str string) error {
	uri, err := url.Parse(str)
	if err != nil {
		return err
	}

	p.Add(&Backend{URL: uri})

	return nil
}

// Add adds a backend to the pool.
// It also starts continuous probing of the backend if the pool is already running.
func (p *BackendPool) Add(b *Backend) {
	p.Lock()
	defer p.Unlock()

	if b.Sick {
		b.Element = p.Sick.PushBack(b)
	} else {
		b.Element = p.Healthy.PushBack(b)
	}

	if p.Changed != nil {
		go b.Run(p.Changed, nil)
	}
}

// Next picks one of the backends and returns.
func (p *BackendPool) Next() *Backend {
	p.Lock()
	defer p.Unlock()

	e := p.Healthy.Front()

	if e == nil {
		return nil
	}

	p.Healthy.MoveToBack(e)

	return e.Value.(*Backend)
}

// Run keeps watching changes of the backends' states to keep Healthy/Sick queues updated.
func (p *BackendPool) Run() {
	if p.Changed != nil { // already running
		return
	}

	p.Changed = make(chan *Backend)

	for e := p.Healthy.Front(); e != nil; e = e.Next() {
		b := e.Value.(*Backend)
		go b.Run(p.Changed, nil)
	}

	for b := range p.Changed {
		log.Printf("backend: %s, sick: %s", b, b.Sick)

		if b.Sick {
			p.Lock()
			b.Element = p.Sick.PushBack(p.Healthy.Remove(b.Element))
			p.Unlock()
		} else {
			p.Lock()
			b.Element = p.Healthy.PushBack(p.Sick.Remove(b.Element))
			p.Unlock()
		}

		log.Printf("backend pool: %s", p)
	}
}
