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

	"github.com/ichiban/jesi/transaction"
)

// Store stores pairs of requests/responses.
type Store struct {
	sync.RWMutex
	Resources       map[ResourceKey]*Resource
	History         *list.List
	Max             int
	OriginChangedAt time.Time
}

// Set inserts/updates a new pair of request/response to the cache.
func (s *Store) Set(req *http.Request, rep *Representation) {
	s.init()

	s.Lock()
	defer s.Unlock()

	resKey := NewResourceKey(req)
	res, ok := s.Resources[resKey]
	if !ok {
		res = NewResource(rep)
		s.Resources[resKey] = res
	}

	repKey := NewRepresentationKey(res, req)
	if old, ok := res.Representations[repKey]; ok && old.Element != nil {
		s.History.Remove(old.Element)

		log.WithFields(log.Fields{
			"id": old.ID,
		}).Info("Removed a representation")
	}
	res.Representations[repKey] = rep

	log.WithFields(log.Fields{
		"id":          rep.ID,
		"transaction": transaction.ID(req),
	}).Info("Added a representation")

	rep.Element = s.History.PushFront(key{resource: resKey, representation: repKey})

	if s.Max == 0 {
		return
	}

	maxInBytes := uint64(s.Max) * 1024 * 1024 // MB

	var stats runtime.MemStats
	for i := 0; i < s.History.Len(); i++ {
		runtime.ReadMemStats(&stats)

		log.WithFields(log.Fields{
			"inuse": stats.HeapInuse,
			"max":   s.Max,
		}).Debug("Read memory stats")

		if stats.HeapInuse < maxInBytes {
			break
		}

		s.evictLRU()
	}
}

func (s *Store) evictLRU() {
	e := s.History.Back()
	if e == nil {
		log.Warn("Couldn't find a cached response to free")
		return
	}
	s.History.Remove(e)
	k, ok := e.Value.(key)
	if !ok {
		log.Fatalf("failed to coerce a history element to key: %#v", e.Value)
	}

	res, ok := s.Resources[k.resource]
	if !ok {
		return
	}

	if rep, ok := res.Representations[k.representation]; ok {
		delete(res.Representations, k.representation)

		log.WithFields(log.Fields{
			"id": rep.ID,
		}).Info("Removed a representation")
	}

	if len(res.Representations) == 0 {
		delete(s.Resources, k.resource)
	}
}

// Get retrieves a cached response.
func (s *Store) Get(req *http.Request) *Representation {
	s.init()

	s.RLock()
	defer s.RUnlock()

	resKey := NewResourceKey(req)
	res, ok := s.Resources[resKey]
	if !ok {
		return nil
	}

	repKey := NewRepresentationKey(res, req)
	rep, ok := res.Representations[repKey]
	if !ok {
		return nil
	}

	if rep.Element != nil {
		s.History.MoveToFront(rep.Element)
	}

	log.WithFields(log.Fields{
		"id": rep.ID,
	}).Info("Get a representation")

	return rep.clone()
}

// Purge removes any representations associated to the request.
func (s *Store) Purge(req *http.Request) {
	s.init()

	s.RLock()
	defer s.RUnlock()

	resKey := NewResourceKey(req)
	res, ok := s.Resources[resKey]
	if !ok {
		return
	}
	delete(s.Resources, resKey)

	for _, rep := range res.Representations {
		s.History.Remove(rep.Element)

		log.WithFields(log.Fields{
			"id": rep.ID,
		}).Info("Removed a representation")
	}
}

func (s *Store) init() {
	s.Lock()
	defer s.Unlock()

	if s.Resources == nil {
		s.Resources = make(map[ResourceKey]*Resource)
	}

	if s.History == nil {
		s.History = list.New()
	}
}

// ResourceKey identifies resources with the same URL.
type ResourceKey struct {
	Host  string `json:"host"`
	Path  string `json:"path"`
	Query string `json:"query"`
}

// NewResourceKey returns a resource key of the request.
func NewResourceKey(req *http.Request) ResourceKey {
	return ResourceKey{
		Host:  req.URL.Host,
		Path:  req.URL.Path,
		Query: req.URL.Query().Encode(),
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

	res := &Resource{
		Fields:          fields,
		Representations: make(map[RepresentationKey]*Representation),
	}

	return res
}

// RepresentationKey identifies a cached response in variations.
type RepresentationKey struct {
	Method string
	Key    string
}

// NewRepresentationKey constructs a new variation key from a request.
func NewRepresentationKey(res *Resource, req *http.Request) RepresentationKey {
	var keys []string
	for _, fields := range res.Fields {
		fields := strings.Split(fields, ",")
		for _, field := range fields {
			keys = append(keys, strings.TrimSpace(field))
		}
	}

	vals := url.Values{}
	for _, key := range keys {
		var values []string
		for _, vals := range req.Header[key] {
			vals := strings.Split(vals, ",")
			for _, val := range vals {
				values = append(values, strings.TrimSpace(val))
			}
		}

		sort.Strings(values)
		for _, val := range values {
			vals.Add(key, val)
		}
	}

	return RepresentationKey{
		Method: req.Method,
		Key:    vals.Encode(),
	}
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
func (s *Store) State(req *http.Request, cached *Representation) (CachedState, time.Duration) {
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
		if time.Since(s.OriginChangedAt) <= age {
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
