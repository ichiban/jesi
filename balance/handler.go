package balance

import (
	"container/list"
	"flag"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/ichiban/jesi/request"
	log "github.com/sirupsen/logrus"
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
		log.WithFields(log.Fields{
			"request": request.ID(r),
		}).Warn("Couldn't find a backend in the pool")

		return
	}

	log.WithFields(log.Fields{
		"request": request.ID(r),
		"backend": b,
	}).Info("Picked up a backend from the pool")

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

	log.WithFields(log.Fields{
		"request": request.ID(r),
		"url":     r.URL,
	}).Info("Directed a request to a backend")
}

// Backend represents an upstream server.
type Backend struct {
	*list.Element
	*url.URL
	http.Client

	Sick     bool
	Interval time.Duration
	Timer    <-chan time.Time
}

func (b *Backend) String() string {
	return b.URL.String()
}

// Run keeps probing the backend to keep its state updated.
// When state changed, it notifies ch.
func (b *Backend) Run(ch chan<- *Backend, q <-chan struct{}) {
	log.Printf("run: %s", b)
	defer log.Printf("done: %s", b)

	if b.Interval == 0 {
		b.Interval = 10 * time.Second
	}

	b.Client.Timeout = 10 * time.Second

	for {
		t := b.Timer
		if t == nil {
			log.Printf("probe after: %s", b.Interval)
			t = time.After(b.Interval)
		}

		select {
		case <-t:
			old := b.Sick
			b.Probe()
			if old != b.Sick {
				ch <- b
			}
		case <-q:
			return
		}
	}
}

// Probe makes a probing request to the background and changes its internal state accordingly.
func (b *Backend) Probe() {
	log.Printf("probe: %s", b)

	resp, err := b.Get(b.URL.String())
	if err != nil {
		log.Printf("error response from backend: %s", b)
		b.Sick = true
		return
	}

	log.Printf("status code: %d from backend: %s", resp.StatusCode, b)

	b.Sick = resp.StatusCode >= 400
}

// BackendPool hold a set of backends.
type BackendPool struct {
	sync.RWMutex

	Changed chan *Backend
	Quit    chan struct{}
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

	if p.Changed == nil {
		p.Changed = make(chan *Backend)
	}

	if p.Quit == nil {
		p.Quit = make(chan struct{})
	}

	if b.Sick {
		b.Element = p.Sick.PushBack(b)
	} else {
		b.Element = p.Healthy.PushBack(b)
	}

	go b.Run(p.Changed, p.Quit)
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
func (p *BackendPool) Run(ch chan struct{}) {
	if ch != nil {
		ch <- struct{}{}
	}

	for {
		select {
		case b := <-p.Changed:
			log.Printf("backend: %s, sick: %t", b, b.Sick)

			p.Lock()
			if b.Sick {
				b.Element = p.Sick.PushBack(p.Healthy.Remove(b.Element))
			} else {
				b.Element = p.Healthy.PushBack(p.Sick.Remove(b.Element))
			}
			p.Unlock()

			if ch != nil {
				ch <- struct{}{}
			}

			log.Printf("backend pool: %s", p)
		case <-ch:
			if p.Quit != nil {
				close(p.Quit)
			}
			return
		}
	}
}
