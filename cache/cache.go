package cache

import (
	"bytes"
	"container/list"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Cache stores pairs of requests/responses.
type Cache struct {
	sync.RWMutex
	URLVars map[URLKey]*Variations
	History *list.List
	Max     uint64
	InUse   uint64
}

// Set inserts/updates a new pair of request/response to the cache.
func (c *Cache) Set(req *http.Request, cached *CachedResponse) {
	c.init()

	c.Lock()
	defer c.Unlock()

	urlKey := NewURLKey(req)
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

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	c.InUse = stats.HeapInuse
	if c.Max != 0 && stats.HeapInuse > c.Max {
		c.removeLRU()
	}
}

func (c *Cache) removeLRU() {
	e := c.History.Back()
	c.History.Remove(e)
	k := e.Value.(key)

	log.Printf("evict: %#v", k)

	pe, ok := c.URLVars[k.primary]
	if !ok {
		return
	}

	_, ok = pe.VarResponse[k.secondary]
	if ok {
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

// Clear removes all request/response pairs which is stored in the cache.
func (c *Cache) Clear() {
	c.Lock()
	defer c.Unlock()

	c.URLVars = make(map[URLKey]*Variations)
	c.History.Init()
}

// URLKey identifies cached responses with the same URL.
type URLKey struct {
	Host  string
	Path  string
	Query string
}

// NewURLKey returns a primary key of the request.
func NewURLKey(req *http.Request) URLKey {
	return URLKey{
		Host:  req.URL.Host,
		Path:  req.URL.Path,
		Query: req.URL.Query().Encode(),
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
func NewCachedResponse(resp *http.Response, reqTime, respTime time.Time) (*CachedResponse, error) {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	resp.Body = ioutil.NopCloser(bytes.NewReader(body))

	return &CachedResponse{
		Header:       resp.Header,
		Body:         body,
		RequestTime:  reqTime,
		ResponseTime: respTime,
	}, nil
}

// Response converts a cached response to an HTTP response.
func (e *CachedResponse) Response() *http.Response {
	h := http.Header{}
	for k, v := range e.Header {
		h[k] = v
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     e.Header,
		Body:       ioutil.NopCloser(bytes.NewReader(e.Body)),
	}
}

type key struct {
	primary   URLKey
	secondary VarKey
}
