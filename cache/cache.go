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

type Cache struct {
	sync.RWMutex
	Primary  map[PrimaryKey]*PrimaryEntry
	History  *list.List
	MaxBytes uint64
}

func (c *Cache) Set(req *http.Request, cached *CachedResponse) error {
	c.init()

	c.Lock()
	defer c.Unlock()

	pKey := NewPrimaryKey(req)
	pe, ok := c.Primary[pKey]
	if !ok {
		pe = NewPrimaryEntry(cached)
		c.Primary[pKey] = pe
	}

	sKey := NewSecondaryKey(pe, req)
	if old, ok := pe.Secondary[sKey]; ok && old.Element != nil {
		c.History.Remove(old.Element)
	}
	pe.Secondary[sKey] = cached
	cached.Element = c.History.PushFront(key{primary: pKey, secondary: sKey})

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	log.Printf("inuse: %d", stats.HeapInuse)
	if c.MaxBytes != 0 && stats.HeapInuse > c.MaxBytes {
		c.removeLRU()
	}

	return nil
}

func (c *Cache) removeLRU() {
	e := c.History.Back()
	c.History.Remove(e)
	k := e.Value.(key)

	log.Printf("evict: %#v", k)

	pe, ok := c.Primary[k.primary]
	if !ok {
		return
	}

	_, ok = pe.Secondary[k.secondary]
	if ok {
		delete(pe.Secondary, k.secondary)
	}

	if len(pe.Secondary) == 0 {
		delete(c.Primary, k.primary)
	}
}

func (c *Cache) Get(req *http.Request) *CachedResponse {
	c.init()

	c.RLock()
	defer c.RUnlock()

	pKey := NewPrimaryKey(req)
	pe, ok := c.Primary[pKey]
	if !ok {
		return nil
	}

	sKey := NewSecondaryKey(pe, req)
	cached, ok := pe.Secondary[sKey]
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

	if c.Primary == nil {
		c.Primary = make(map[PrimaryKey]*PrimaryEntry)
	}

	if c.History == nil {
		c.History = list.New()
	}
}

func (c *Cache) Clear() {
	c.Lock()
	defer c.Unlock()

	c.Primary = make(map[PrimaryKey]*PrimaryEntry)
	c.History.Init()
}

type PrimaryKey struct {
	Host  string
	Path  string
	Query string
}

func NewPrimaryKey(req *http.Request) PrimaryKey {
	return PrimaryKey{
		Host:  req.URL.Host,
		Path:  req.URL.Path,
		Query: req.URL.Query().Encode(),
	}
}

type PrimaryEntry struct {
	Fields    []string
	Secondary map[SecondaryKey]*CachedResponse
}

func NewPrimaryEntry(resp *CachedResponse) *PrimaryEntry {
	var fields []string

	for _, vary := range resp.Header["Vary"] {
		for _, field := range strings.Split(vary, ",") {
			if field == "*" {
				return nil
			}
			fields = append(fields, http.CanonicalHeaderKey(strings.Trim(field, " ")))
		}
	}

	return &PrimaryEntry{
		Fields:    fields,
		Secondary: make(map[SecondaryKey]*CachedResponse),
	}
}

type SecondaryKey string

func NewSecondaryKey(pe *PrimaryEntry, req *http.Request) SecondaryKey {
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

	return SecondaryKey(vals.Encode())
}

type CachedResponse struct {
	sync.RWMutex
	Header       http.Header
	Body         []byte
	RequestTime  time.Time
	ResponseTime time.Time
	Element      *list.Element
}

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
	primary   PrimaryKey
	secondary SecondaryKey
}
