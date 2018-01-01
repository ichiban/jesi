package cache

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"

	"github.com/ichiban/jesi/transaction"
)

// Store stores pairs of request/response.
type Store struct {
	sync.RWMutex
	Resources       map[ResourceKey]*Resource
	Representations map[uuid.UUID]*Representation
	Max             uint64
	InUse           uint64
	Sample          uint
	OriginChangedAt time.Time
}

// Set inserts/updates a new pair of request/response to the cache.
func (s *Store) Set(req *http.Request, rep *Representation) {
	s.Init()

	s.Lock()
	defer s.Unlock()

	resKey := NewResourceKey(req)
	res, ok := s.Resources[resKey]
	if !ok {
		res = NewResource(req, rep)
		s.Resources[resKey] = res
	}

	repKey := NewRepresentationKey(res, req)
	if old, ok := res.Representations[repKey]; ok {
		s.InUse -= uint64(len(old.Body))
		log.WithFields(log.Fields{
			"id": old.ID,
		}).Info("Removed a representation")
	}
	res.Representations[repKey] = rep
	rep.ResourceKey = resKey
	rep.RepresentationKey = repKey
	s.Representations[rep.ID] = rep
	rep.LastUsedTime = time.Now()

	log.WithFields(log.Fields{
		"id":          rep.ID,
		"transaction": transaction.ID(req),
	}).Info("Added a representation")

	s.InUse += uint64(len(rep.Body))

	if s.Max == 0 {
		return
	}

	for s.InUse > s.Max {
		s.evict()
	}
}

func (s *Store) evict() {
	var minID uuid.UUID
	var minRep *Representation
	i := s.Sample
	for id, rep := range s.Representations {
		if i == 0 {
			break
		}

		if minRep != nil && less(minRep, rep) {
			continue
		}

		minID = id
		minRep = rep

		i--
	}

	res := s.Resources[minRep.ResourceKey]
	delete(res.Representations, minRep.RepresentationKey)
	if len(res.Representations) == 0 {
		delete(s.Resources, res.ResourceKey)
	}

	delete(s.Representations, minID)
	s.InUse -= uint64(len(minRep.Body))

	log.WithFields(log.Fields{
		"id": minRep.ID,
	}).Info("Removed a representation")
}

// Get retrieves a cached response.
func (s *Store) Get(req *http.Request) *Representation {
	s.Init()

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

	rep.Lock()
	defer rep.Unlock()
	rep.LastUsedTime = time.Now()

	log.WithFields(log.Fields{
		"id": rep.ID,
	}).Info("Get a representation")

	return rep.clone()
}

// Purge removes any representations associated to the request.
func (s *Store) Purge(req *http.Request) *Resource {
	s.Init()

	s.Lock()
	defer s.Unlock()

	resKey := NewResourceKey(req)
	res, ok := s.Resources[resKey]
	if !ok {
		return nil
	}
	delete(s.Resources, resKey)

	for repKey := range res.Representations {
		rep, ok := res.Representations[repKey]
		if !ok {
			continue
		}
		s.InUse -= uint64(len(rep.Body))

		log.WithFields(log.Fields{
			"id": rep.ID,
		}).Info("Removed a representation")
	}

	return res
}

func (s *Store) Init() {
	s.Lock()
	defer s.Unlock()

	if s.Resources == nil {
		s.Resources = make(map[ResourceKey]*Resource)
	}
	if s.Representations == nil {
		s.Representations = make(map[uuid.UUID]*Representation)
		for _, res := range s.Resources {
			for _, rep := range res.Representations {
				s.Representations[rep.ID] = rep
			}
		}
	}
}

// ResourceKey identifies a resource.
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

// Resource represents a set of representations with the same URL.
type Resource struct {
	ResourceKey
	Unique          bool
	Fields          []string
	Representations map[RepresentationKey]*Representation
}

// NewResource constructs a resource from a representation.
func NewResource(req *http.Request, rep *Representation) *Resource {
	res := Resource{
		ResourceKey:     NewResourceKey(req),
		Representations: make(map[RepresentationKey]*Representation),
	}

	for _, vary := range rep.HeaderMap["Vary"] {
		vary = strings.TrimSpace(vary)
		if vary == "*" {
			res.Unique = true
			res.Fields = nil
			return &res
		}
		for _, field := range strings.Split(vary, ",") {
			res.Fields = append(res.Fields, http.CanonicalHeaderKey(strings.TrimSpace(field)))
		}
	}

	return &res
}

// RepresentationKey identifies a representation in a resource.
type RepresentationKey struct {
	Method string
	Key    string
}

// NewRepresentationKey constructs a representation key from a request.
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

func less(a, b *Representation) bool {
	return a.LastUsedTime.Before(b.LastUsedTime)
}
