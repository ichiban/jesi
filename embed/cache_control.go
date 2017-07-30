package embed

import (
	"fmt"
	"github.com/ichiban/jesi/cache"
	"strconv"
	"strings"
	"time"
)

const (
	dateField = "Date"
)

const (
	mustRevalidateDirective = "must-revalidate"
	noCacheDirective        = "no-cache"
	noStoreDirective        = "no-store"
	publicDirective         = "public"
	privateDirective        = "private"
	immutableDirective      = "immutable"
	maxAgeDirective         = "max-age"
)

// CacheControl represents a representation's cache policy.
type CacheControl struct {
	MustRevalidate bool
	NoCache        bool
	NoStore        bool
	Public         bool
	Private        bool
	Immutable      bool
	MaxAge         *time.Duration
}

// NewCacheControl creates a new instance of CacheControl from related headers in the given HTTP response.
func NewCacheControl(rep *cache.Representation) *CacheControl {
	c := newControlExpires(rep)

	for _, d := range directives(rep) {
		ts := strings.Split(d, "=")
		switch ts[0] {
		case mustRevalidateDirective:
			c.MustRevalidate = true
		case noCacheDirective:
			c.NoCache = true
		case noStoreDirective:
			c.NoStore = true
		case publicDirective:
			c.Public = true
		case privateDirective:
			c.Private = true
		case immutableDirective:
			c.Immutable = true
		case maxAgeDirective:
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
func newControlExpires(rep *cache.Representation) *CacheControl {
	var c CacheControl

	e, ok := rep.HeaderMap[expiresField]
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

	d, ok := rep.HeaderMap[dateField]
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
	for _, v := range rep.HeaderMap[cacheControlField] {
		for _, d := range strings.Split(v, ",") {
			d = strings.Trim(d, " ")
			ds = append(ds, d)
		}
	}
	return ds
}

// Merge merges 2 Controls.
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
		ds = append(ds, mustRevalidateDirective)
	}

	if c.NoCache {
		ds = append(ds, noCacheDirective)
	}

	if c.NoStore {
		ds = append(ds, noStoreDirective)
	}

	if c.Public {
		ds = append(ds, publicDirective)
	}

	if c.Private {
		ds = append(ds, privateDirective)
	}

	if c.Immutable {
		ds = append(ds, immutableDirective)
	}

	if c.MaxAge != nil {
		ds = append(ds, fmt.Sprintf("%s=%d", maxAgeDirective, *c.MaxAge/time.Second))
	}

	return strings.Join(ds, ",")
}
