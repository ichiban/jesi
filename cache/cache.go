package cache

import (
	"container/list"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ichiban/jesi/common"
)

// Cache stores pairs of requests/responses.
type Cache struct {
	sync.RWMutex
	URLVars         map[URLKey]*Variations
	History         *list.List
	Max             uint64
	OriginChangedAt time.Time
}

// Set inserts/updates a new pair of request/response to the cache.
func (c *Cache) Set(req *http.Request, cached *CachedResponse) {
	c.init()

	c.Lock()
	defer c.Unlock()

	urlKey := NewURLKey(req)
	log.Printf("set: %#v", urlKey)

	variations, ok := c.URLVars[urlKey]
	if !ok {
		variations = NewVariations(cached)
		c.URLVars[urlKey] = variations
	}

	varKey := NewVarKey(variations, req)
	if old, ok := variations.VarResponse[varKey]; ok && old.Element != nil {
		c.History.Remove(old.Element)
	}
	variations.VarResponse[varKey] = cached
	cached.Element = c.History.PushFront(key{primary: urlKey, secondary: varKey})

	if c.Max == 0 {
		return
	}

	var stats runtime.MemStats
	for i := 0; i < c.History.Len(); i++ {
		runtime.ReadMemStats(&stats)
		log.Printf("inuse: %dB, max: %dB\n", stats.HeapInuse, c.Max)
		if stats.HeapInuse < c.Max {
			break
		}
		c.removeLRU()
	}
}

func (c *Cache) removeLRU() {
	e := c.History.Back()
	if e == nil {
		log.Print("no cached response to free")
		return
	}
	c.History.Remove(e)
	k := e.Value.(key)

	log.Printf("evict: %#v", k)

	pe, ok := c.URLVars[k.primary]
	if !ok {
		return
	}

	if _, ok = pe.VarResponse[k.secondary]; ok {
		delete(pe.VarResponse, k.secondary)
	}

	if len(pe.VarResponse) == 0 {
		delete(c.URLVars, k.primary)
	}
}

// Get retrieves a cached response.
func (c *Cache) Get(req *http.Request) *CachedResponse {
	c.init()

	c.RLock()
	defer c.RUnlock()

	pKey := NewURLKey(req)
	pe, ok := c.URLVars[pKey]
	if !ok {
		return nil
	}

	sKey := NewVarKey(pe, req)
	cached, ok := pe.VarResponse[sKey]
	if !ok {
		return nil
	}

	if cached.Element != nil {
		c.History.MoveToFront(cached.Element)
	}

	return cached
}

func (c *Cache) init() {
	c.Lock()
	defer c.Unlock()

	if c.URLVars == nil {
		c.URLVars = make(map[URLKey]*Variations)
	}

	if c.History == nil {
		c.History = list.New()
	}
}

// URLKey identifies cached responses with the same URL.
type URLKey struct {
	Method string
	Host   string
	Path   string
	Query  string
}

// NewURLKey returns a primary key of the request.
func NewURLKey(req *http.Request) URLKey {
	return URLKey{
		Method: req.Method,
		Host:   req.URL.Host,
		Path:   req.URL.Path,
		Query:  req.URL.Query().Encode(),
	}
}

// Variations represents cached responses with the same URL.
type Variations struct {
	Fields      []string
	VarResponse map[VarKey]*CachedResponse
}

// NewVariations constructs a new variations for the cached response.
func NewVariations(resp *CachedResponse) *Variations {
	var fields []string

	for _, vary := range resp.Header["Vary"] {
		for _, field := range strings.Split(vary, ",") {
			if field == "*" {
				return nil
			}
			fields = append(fields, http.CanonicalHeaderKey(strings.Trim(field, " ")))
		}
	}

	return &Variations{
		Fields:      fields,
		VarResponse: make(map[VarKey]*CachedResponse),
	}
}

// VarKey identifies a cached response in variations.
type VarKey string

// NewVarKey constructs a new variation key from a request.
func NewVarKey(pe *Variations, req *http.Request) VarKey {
	var keys []string
	for _, fields := range pe.Fields {
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

	return VarKey(vals.Encode())
}

// CachedResponse represents a cached HTTP response.
type CachedResponse struct {
	sync.RWMutex
	Header       http.Header
	Body         []byte
	RequestTime  time.Time
	ResponseTime time.Time
	Element      *list.Element
}

// NewCachedResponse constructs a new cached response.
func NewCachedResponse(resp *common.ResponseBuffer, reqTime, respTime time.Time) (*CachedResponse, error) {
	return &CachedResponse{
		Header:       resp.HeaderMap,
		Body:         resp.Body,
		RequestTime:  reqTime,
		ResponseTime: respTime,
	}, nil
}

// Response converts a cached response to an HTTP response.
func (e *CachedResponse) Response() *common.ResponseBuffer {
	return &common.ResponseBuffer{
		StatusCode: http.StatusOK,
		HeaderMap:  e.Header,
		Body:       e.Body,
	}
}

type key struct {
	primary   URLKey
	secondary VarKey
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
func (c *Cache) State(req *http.Request, cached *CachedResponse) (CachedState, time.Duration) {
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

	if contains(cached.Header, cacheControlField, noStore) {
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

		if contains(cached.Header, cacheControlField, revalidatePattern) {
			return Revalidate, time.Duration(0)
		}

		return Stale, delta
	}

	return Revalidate, time.Duration(0)
}
