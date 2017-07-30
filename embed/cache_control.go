package embed

import (
	"fmt"
	"github.com/ichiban/jesi/cache"
	"strconv"
	"strings"
	"time"
)

// CacheControl represents a response's cache policy.
type CacheControl struct {
	MustRevalidate bool
	NoCache        bool
	NoStore        bool
	Public         bool
	Private        bool
	Immutable      bool
	MaxAge         *time.Duration
}

const (
	expires      = "Expires"
	date         = "Date"
	cacheControl = "Cache-Control"

	mustRevalidate = "must-revalidate"
	noCache        = "no-cache"
	noStore        = "no-store"
	public         = "public"
	private        = "private"
	immutable      = "immutable"
	maxAge         = "max-age"
)

// NewCacheControl creates a new instance of CacheControl from related headers in the given HTTP response.
func NewCacheControl(rep *cache.Representation) *CacheControl {
	c := newCacheControlExpires(rep)

	for _, d := range directives(rep) {
		ts := strings.Split(d, "=")
		switch ts[0] {
		case mustRevalidate:
			c.MustRevalidate = true
		case noCache:
			c.NoCache = true
		case noStore:
			c.NoStore = true
		case public:
			c.Public = true
		case private:
			c.Private = true
		case immutable:
			c.Immutable = true
		case maxAge:
			n, err := strconv.Atoi(ts[1])
			if err != nil {
				continue
			}
			a := time.Duration(n) * time.Second
			c.MaxAge = &a
		}
	}

	return c
}

// Convert Expires to Cache-Control: max-age
func newCacheControlExpires(rep *cache.Representation) *CacheControl {
	var c CacheControl

	e, ok := rep.HeaderMap[expires]
	if !ok {
		return &c
	}

	t, err := time.Parse(time.RFC1123, e[0])
	if err != nil {
		// Treat invalid Expires as expired.
		a := time.Duration(0)
		c.MaxAge = &a
		return &c
	}

	d, ok := rep.HeaderMap[date]
	if !ok {
		return &c
	}

	s, err := time.Parse(time.RFC1123, d[0])
	if err != nil {
		return &c
	}

	a := t.Sub(s)
	c.MaxAge = &a

	return &c
}

// TODO: Proper directive parsing.
// Cache-Control directives can be something like `private="foo,bar,baz"`.
func directives(rep *cache.Representation) []string {
	ds := []string{}
	for _, v := range rep.HeaderMap[cacheControl] {
		for _, d := range strings.Split(v, ",") {
			d = strings.Trim(d, " ")
			ds = append(ds, d)
		}
	}
	return ds
}

// Merge merges 2 CacheControls.
func (c *CacheControl) Merge(o *CacheControl) *CacheControl {
	var n CacheControl

	n.MustRevalidate = c.MustRevalidate || o.MustRevalidate
	n.NoCache = c.NoCache || o.NoCache
	n.NoStore = c.NoStore || o.NoStore
	n.Public = c.Public && o.Public
	n.Private = c.Private || o.Private
	n.Immutable = c.Immutable && o.Immutable
	n.MaxAge = c.MaxAge
	if n.MaxAge != nil && o.MaxAge != nil && *n.MaxAge > *o.MaxAge {
		n.MaxAge = o.MaxAge
	}

	return &n
}

// String generates CacheControl header value.
func (c *CacheControl) String() string {
	var ds []string

	if c.MustRevalidate {
		ds = append(ds, mustRevalidate)
	}

	if c.NoCache {
		ds = append(ds, noCache)
	}

	if c.NoStore {
		ds = append(ds, noStore)
	}

	if c.Public {
		ds = append(ds, public)
	}

	if c.Private {
		ds = append(ds, private)
	}

	if c.Immutable {
		ds = append(ds, immutable)
	}

	if c.MaxAge != nil {
		ds = append(ds, fmt.Sprintf("%s=%d", maxAge, *c.MaxAge/time.Second))
	}

	return strings.Join(ds, ",")
}
