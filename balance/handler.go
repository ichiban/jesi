package balance

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/ichiban/jesi/transaction"
)

const (
	xForwardedFor   = "X-Forwarded-For"
	xForwardedHost  = "X-Forwarded-Host"
	xForwardedProto = "X-Forwarded-Proto"
	forwarded       = "Forwarded"
	connection      = "Connection"
)

var (
	hopByHopFields = regexp.MustCompile(`\A(?i:Connection|Keep-Alive|Proxy-Authenticate|Proxy-Authorization|TE|Trailer|Transfer-Encoding|Upgrade)\z`)
	nodePattern    = regexp.MustCompile(`\A(.+?)(?::(\d*))?\z`)
	tchar          = regexp.MustCompile("\\A[!#$%&'*+\\-.^_`\\|~[:alnum:]]*\\z")
	obfuscated     = regexp.MustCompile(`\A_[[:alnum:]._-]*\z`)
)

// Handler is a reverse proxy with multiple backends.
type Handler struct {
	*Node
	*BackendPool

	Next http.Handler
}

var _ http.Handler = (*Handler)(nil)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = cloneReq(r)
	if err := h.direct(r); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	removeHopByHops(r.Header)
	h.addForwarded(r)
	addXForwarded(r)
	h.Next.ServeHTTP(w, r)
	removeHopByHops(w.Header())
}

var errBackendNotFound = errors.New("backend not found")

func (h *Handler) direct(r *http.Request) error {
	b := h.BackendPool.Next()

	if b == nil {
		log.WithFields(log.Fields{
			"id": transaction.ID(r),
		}).Error("Couldn't find a backend in the pool")

		return errBackendNotFound
	}

	log.WithFields(log.Fields{
		"id":      transaction.ID(r),
		"backend": b,
	}).Debug("Picked up a backend from the pool")

	r.URL.Scheme = b.URL.Scheme
	r.URL.Host = b.URL.Host
	r.URL.Path = path.Join(b.URL.Path, r.URL.Path)
	if b.URL.RawQuery == "" || r.URL.RawQuery == "" {
		r.URL.RawQuery = b.URL.RawQuery + r.URL.RawQuery
	} else {
		r.URL.RawQuery = b.URL.RawQuery + "&" + r.URL.RawQuery
	}

	log.WithFields(log.Fields{
		"id":  transaction.ID(r),
		"url": r.URL,
	}).Debug("Directed a request to a backend")

	return nil
}

func cloneReq(old *http.Request) *http.Request {
	r := &http.Request{}
	*r = *old
	r.Header = http.Header{}
	for k, vs := range old.Header {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	return r
}

func removeHopByHops(h http.Header) {
	var fields []string
	cs, ok := h[connection]
	if ok {
		for _, c := range cs {
			fs := strings.Split(c, ",")
			for _, f := range fs {
				fields = append(fields, strings.TrimSpace(f))
			}
		}
	}

	for k := range h {
		if hopByHopFields.MatchString(k) {
			h.Del(k)
			continue
		}
		for _, f := range fields {
			if strings.EqualFold(k, f) {
				h.Del(k)
				break
			}
		}
	}
}

func addXForwarded(r *http.Request) {
	addXForwardedFor(r)
	addXForwardedHost(r)
	addXForwardedProto(r)
}

func addXForwardedFor(r *http.Request) {
	if r.RemoteAddr == "" {
		return
	}
	addrs := r.Header[xForwardedFor]
	if n, err := ParseNode(r.RemoteAddr); err == nil {
		n.Port = nil // omit port
		addrs = append(addrs, n.String())
	}
	r.Header.Set(xForwardedFor, strings.Join(addrs, ", "))
}

func addXForwardedHost(r *http.Request) {
	if _, ok := r.Header[xForwardedHost]; ok {
		return
	}

	if r.Host == "" {
		return
	}

	r.Header.Set(xForwardedHost, r.Host)
}

func addXForwardedProto(r *http.Request) {
	if _, ok := r.Header[xForwardedProto]; ok {
		return
	}

	var proto string
	if r.TLS == nil {
		proto = "http"
	} else {
		proto = "https"
	}

	r.Header.Set(xForwardedProto, proto)
}

// https://tools.ietf.org/html/rfc7239
func (h *Handler) addForwarded(r *http.Request) {
	convertXForwardedFor(r)

	e, err := h.newElement(r)
	if err != nil {
		return
	}
	if e != nil {
		r.Header[forwarded] = append(r.Header[forwarded], e.String())
	}
}

type element struct {
	By    *Node
	For   *Node
	Host  string
	Proto string
}

func (h *Handler) newElement(r *http.Request) (*element, error) {
	var e element

	if h.Node != nil {
		e.By = h.Node
	}

	if r.RemoteAddr != "" {
		n, err := ParseNode(r.RemoteAddr)
		if err != nil {
			return nil, err
		}
		e.For = n
	}

	if r.Host != "" {
		e.Host = r.Host
	}

	if r.TLS == nil {
		e.Proto = "http"
	} else {
		e.Proto = "https"
	}

	return &e, nil
}

func (e *element) String() string {
	var pairs []string
	if e.By != nil {
		pairs = append(pairs, fmt.Sprintf(`by=%s`, tokenOrQuotedString(e.By.String())))
	}
	if e.For != nil {
		pairs = append(pairs, fmt.Sprintf(`for=%s`, tokenOrQuotedString(e.For.String())))
	}
	if e.Host != "" {
		pairs = append(pairs, fmt.Sprintf(`host=%s`, tokenOrQuotedString(e.Host)))
	}
	if e.Proto != "" {
		pairs = append(pairs, fmt.Sprintf(`proto=%s`, tokenOrQuotedString(e.Proto)))
	}
	return strings.Join(pairs, ";")
}

func tokenOrQuotedString(s string) string {
	if tchar.MatchString(s) {
		return s
	}
	return fmt.Sprintf(`"%s"`, s)
}

func convertXForwardedFor(r *http.Request) {
	// the downstream is already using Forwarded, assume conversion is done.
	if _, ok := r.Header[forwarded]; ok {
		return
	}

	vs := r.Header[xForwardedFor]

	var ns []*Node
	for _, v := range vs {
		pairs := strings.Split(v, ",")
		for _, pair := range pairs {
			n, err := ParseNode(strings.TrimSpace(pair))
			if err != nil {
				return
			}
			ns = append(ns, n)
		}
	}

	for _, n := range ns {
		r.Header[forwarded] = append(r.Header[forwarded], (&element{For: n}).String())
	}
}
