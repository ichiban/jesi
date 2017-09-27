package forward

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
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

// Handler forwards requests to backends.
type Handler struct {
	Transport http.RoundTripper
}

var _ http.Handler = (*Handler)(nil)

// ServeHTTP forwards requests to the backend. https://tools.ietf.org/html/rfc7239
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = cloneReq(r)
	removeHopByHops(r.Header)
	addForwarded(r)
	addXForwarded(r)

	res, err := h.Transport.RoundTrip(r)
	if err != nil {
		log.WithFields(log.Fields{
			"id":    transaction.ID(r),
			"error": err,
		}).Error("failed to forward a request")

		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() {
		if res.Body == nil {
			return
		}
		if err := res.Body.Close(); err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("failed to close a response body")
		}
	}()

	copyHeader(w.Header(), res.Header)
	removeHopByHops(w.Header())

	w.WriteHeader(res.StatusCode)
	if res.Body != nil {
		if _, err := io.Copy(w, res.Body); err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("failed to copy a response body")
		}
	}

	copyHeader(w.Header(), res.Trailer)
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

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
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
	if n, err := parseNode(r.RemoteAddr); err == nil {
		n.port = nil // omit port
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

func addForwarded(r *http.Request) {
	convertXForwardedFor(r)

	e, err := newElement(r)
	if err != nil {
		return
	}
	if e != nil {
		r.Header[forwarded] = append(r.Header[forwarded], e.String())
	}
}

type element struct {
	By    *node
	For   *node
	Host  string
	Proto string
}

func newElement(r *http.Request) (*element, error) {
	var e element

	// TODO: by

	if r.RemoteAddr != "" {
		n, err := parseNode(r.RemoteAddr)
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
	// the downstream agent is already using Forwarded, assume conversion is done.
	if _, ok := r.Header[forwarded]; ok {
		return
	}

	vs := r.Header[xForwardedFor]

	var ns []*node
	for _, v := range vs {
		pairs := strings.Split(v, ",")
		for _, pair := range pairs {
			n, err := parseNode(strings.TrimSpace(pair))
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

type node struct {
	ident string
	ip    net.IP
	port  *int
}

func parseNode(s string) (*node, error) {
	m := nodePattern.FindStringSubmatch(s)
	name := m[1]
	port := m[2]

	var n node
	if port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
		n.port = &p
	}

	// "unknown" Identifier
	if "unknown" == name {
		n.ident = name
		return &n, nil
	}

	// Obfuscated Identifier
	if obfuscated.MatchString(name) {
		n.ident = name
		return &n, nil
	}

	// strip []
	if strings.HasPrefix(name, `[`) {
		name = name[1 : len(name)-1]
	}

	n.ip = net.ParseIP(name)
	if n.ip == nil {
		return nil, fmt.Errorf("failed to parse: %s", name)
	}
	return &n, nil
}

func (n *node) String() string {
	var s string

	if n.ident != "" {
		s = n.ident
	} else {
		s = n.ip.String()
		if n.ip.To4() == nil { // v6
			s = fmt.Sprintf("[%s]", s)
		}
	}

	if n.port != nil {
		s = fmt.Sprintf("%s:%d", s, *n.port)
	}

	return s
}
