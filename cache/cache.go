package cache

import (
	"container/list"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Cache stores pairs of requests/responses.
type Cache struct {
	sync.RWMutex
	Resource        map[ResourceKey]*Resource
	History         *list.List
	Max             uint64
	OriginChangedAt time.Time
}

// Set inserts/updates a new pair of request/response to the cache.
func (c *Cache) Set(req *http.Request, cached *Representation) {
	c.init()

	c.Lock()
	defer c.Unlock()

	urlKey := NewResourceKey(req)
	log.WithFields(log.Fields{
		"method": urlKey.Method,
		"host":   urlKey.Host,
		"path":   urlKey.Path,
		"query":  urlKey.Query,
	}).Debug("Will set a set of variations to a URL key")

	variations, ok := c.Resource[urlKey]
	if !ok {
		variations = NewResource(cached)
		c.Resource[urlKey] = variations
	}

	varKey := NewRepresentationKey(variations, req)
	log.WithFields(log.Fields{
		"variation": varKey,
	}).Debug("Will set a cached response to a variation key")

	if old, ok := variations.Representations[varKey]; ok && old.Element != nil {
		c.History.Remove(old.Element)

		log.WithFields(log.Fields{
			"method":    urlKey.Method,
			"host":      urlKey.Host,
			"path":      urlKey.Path,
			"query":     urlKey.Query,
			"variation": varKey,
		}).Debug("Removed an old cached response from the history")
	}
	variations.Representations[varKey] = cached
	cached.Element = c.History.PushFront(key{resource: urlKey, representation: varKey})

	if c.Max == 0 {
		return
	}

	var stats runtime.MemStats
	for i := 0; i < c.History.Len(); i++ {
		runtime.ReadMemStats(&stats)

		log.WithFields(log.Fields{
			"inuse": stats.HeapInuse,
			"max":   c.Max,
		}).Info("Read memory stats")

		if stats.HeapInuse < c.Max {
			break
		}

		c.evictLRU()
	}
}

func (c *Cache) evictLRU() {
	e := c.History.Back()
	if e == nil {
		log.Warn("Couldn't find a cached response to free")
		return
	}
	c.History.Remove(e)
	k := e.Value.(key)

	log.WithFields(log.Fields{
		"method":    k.resource.Method,
		"host":      k.resource.Host,
		"path":      k.resource.Path,
		"query":     k.resource.Query,
		"variation": k.representation,
	}).Debug("Will evict a key from the cache")

	pe, ok := c.Resource[k.resource]
	if !ok {
		return
	}

	if _, ok = pe.Representations[k.representation]; ok {
		delete(pe.Representations, k.representation)
	}

	if len(pe.Representations) == 0 {
		delete(c.Resource, k.resource)
	}

	log.WithFields(log.Fields{
		"method":    k.resource.Method,
		"host":      k.resource.Host,
		"path":      k.resource.Path,
		"query":     k.resource.Query,
		"variation": k.representation,
	}).Info("Evicted a key from the cache")
}

// Get retrieves a cached response.
func (c *Cache) Get(req *http.Request) *Representation {
	c.init()

	c.RLock()
	defer c.RUnlock()

	pKey := NewResourceKey(req)
	log.WithFields(log.Fields{
		"method": pKey.Method,
		"host":   pKey.Host,
		"path":   pKey.Path,
		"query":  pKey.Query,
	}).Debug("Will get a set of variations for a URL key")

	pe, ok := c.Resource[pKey]
	if !ok {
		log.WithFields(log.Fields{
			"method": pKey.Method,
			"host":   pKey.Host,
			"path":   pKey.Path,
			"query":  pKey.Query,
		}).Debug("Couldn't get a set of variations for a URL key")

		return nil
	}

	sKey := NewRepresentationKey(pe, req)
	log.WithFields(log.Fields{
		"variation": sKey,
	}).Debug("Will get a cached response for a variation key")

	cached, ok := pe.Representations[sKey]
	if !ok {
		log.WithFields(log.Fields{
			"method":    pKey.Method,
			"host":      pKey.Host,
			"path":      pKey.Path,
			"query":     pKey.Query,
			"variation": sKey,
		}).Debug("Couldn't get a cached response for the key")

		return nil
	}

	if cached.Element != nil {
		c.History.MoveToFront(cached.Element)

		log.WithFields(log.Fields{
			"method":    pKey.Method,
			"host":      pKey.Host,
			"path":      pKey.Path,
			"query":     pKey.Query,
			"variation": sKey,
		}).Debug("Marked a cached response as recently used")
	}

	log.WithFields(log.Fields{
		"method":    pKey.Method,
		"host":      pKey.Host,
		"path":      pKey.Path,
		"query":     pKey.Query,
		"variation": sKey,
	}).Debug("Got a cached response for the key")

	return cached.clone()
}

func (c *Cache) init() {
	c.Lock()
	defer c.Unlock()

	if c.Resource == nil {
		c.Resource = make(map[ResourceKey]*Resource)
	}

	if c.History == nil {
		c.History = list.New()
	}
}

// ResourceKey identifies resources with the same URL.
type ResourceKey struct {
	Method string
	Host   string
	Path   string
	Query  string
}

// NewResourceKey returns a resource key of the request.
func NewResourceKey(req *http.Request) ResourceKey {
	return ResourceKey{
		Method: req.Method,
		Host:   req.URL.Host,
		Path:   req.URL.Path,
		Query:  req.URL.Query().Encode(),
	}
}

// Resource represents cached responses with the same URL.
type Resource struct {
	Fields          []string
	Representations map[RepresentationKey]*Representation
}

// NewResource constructs a new variations for the cached response.
func NewResource(rep *Representation) *Resource {
	var fields []string

	for _, vary := range rep.HeaderMap["Vary"] {
		for _, field := range strings.Split(vary, ",") {
			if field == "*" {
				return nil
			}
			fields = append(fields, http.CanonicalHeaderKey(strings.Trim(field, " ")))
		}
	}

	return &Resource{
		Fields:          fields,
		Representations: make(map[RepresentationKey]*Representation),
	}
}

// RepresentationKey identifies a cached response in variations.
type RepresentationKey string

// NewRepresentationKey constructs a new variation key from a request.
func NewRepresentationKey(res *Resource, req *http.Request) RepresentationKey {
	var keys []string
	for _, fields := range res.Fields {
		fields := strings.Split(fields, ",")
		for _, field := range fields {
			keys = append(keys, strings.Trim(field, " "))
		}
	}

	vals := url.Values{}
	for _, key := range keys {
		var values []string
		for _, vals := range req.Header[key] {
			vals := strings.Split(vals, ",")
			for _, val := range vals {
				values = append(values, strings.Trim(val, " "))
			}
		}

		sort.Strings(values)
		for _, val := range values {
			vals.Add(key, val)
		}
	}

	return RepresentationKey(vals.Encode())
}

type key struct {
	resource       ResourceKey
	representation RepresentationKey
}

// CachedState represents freshness of a cached response.
type CachedState int

const (
	// Miss means it's not in the cache.
	Miss CachedState = iota

	// Fresh means it has a cached response and it's available.
	Fresh

	// Stale means it has a cached response but it's not recommended.
	Stale

	// Revalidate means it has a cached response but needs confirmation from the backend.
	Revalidate
)

// State returns the state of cached response.
func (c *Cache) State(req *http.Request, cached *Representation) (CachedState, time.Duration) {
	if cached == nil {
		return Miss, time.Duration(0)
	}

	cached.RLock()
	defer cached.RUnlock()

	if contains(req.Header, pragmaField, noStore) {
		return Revalidate, time.Duration(0)
	}

	if contains(req.Header, cacheControlField, noStore) {
		return Revalidate, time.Duration(0)
	}

	if contains(cached.HeaderMap, cacheControlField, noStore) {
		return Revalidate, time.Duration(0)
	}

	if lifetime, ok := freshnessLifetime(cached); ok {
		age := currentAge(cached)

		// cached responses before the last destructive requests (e.g. POST) are considered outdated.
		if time.Since(c.OriginChangedAt) <= age {
			return Revalidate, time.Duration(0)
		}

		delta := age - lifetime
		if lifetime > age {
			return Fresh, delta
		}

		if contains(cached.HeaderMap, cacheControlField, revalidatePattern) {
			return Revalidate, time.Duration(0)
		}

		return Stale, delta
	}

	return Revalidate, time.Duration(0)
}

func (s CachedState) String() string {
	switch s {
	case Miss:
		return "miss"
	case Fresh:
		return "fresh"
	case Stale:
		return "stale"
	case Revalidate:
		return "revalidate"
	default:
		return "unknown"
	}
}
