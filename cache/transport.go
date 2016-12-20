package cache

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	pragmaField          = "Pragma"
	cacheControlField    = "Cache-Control"
	expiresField         = "Expires"
	authorizationField   = "Authorization"
	dateField            = "Date"
	ageField             = "Age"
	warningField         = "Warning"
	etagField            = "ETag"
	ifNoneMatchField     = "If-None-Match"
	lastModifiedField    = "Last-Modified"
	ifModifiedSinceField = "If-Modified-Since"
)

var (
	noCache                         = regexp.MustCompile(`\Ano-cache\z`)
	noCacheOrPrivate                = regexp.MustCompile(`\A(?:no-cache|private)\z`)
	mustRevalidateOrPublicOrSMaxage = regexp.MustCompile(`\A(?:must-revalidate|public|s-maxage=\d+)\z`)
	maxAgeOrSMaxageOrPublic         = regexp.MustCompile(`\A(?:max-age=\d+|s-maxage=\d+|public)\z`)

	sMaxagePattern = regexp.MustCompile(`\As-maxage=(\d+)\z`)
	maxAgePattern  = regexp.MustCompile(`\Amax-age=(\d+)\z`)

	revalidatePattern = regexp.MustCompile(`\A(?:s-maxage=\d+|(?:must|proxy)-revalidate)\z`)

	maxStalePattern = regexp.MustCompile(`\Amax-stale=(\d+)\z`)
)

// Transport is a caching round tripper.
type Transport struct {
	http.RoundTripper
	Cache
}

var _ http.RoundTripper = (*Transport)(nil)

// RoundTrip returns a cached response if found. Otherwise, retrieve one from the underlying transport.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.init()

	if !idempotent(req) {
		return t.roundTripClear(req)
	}

	cached := t.Get(req)
	if cached != nil {
		cached.RLock()
		defer cached.RUnlock()
	}

	state, delta := State(req, cached)
	switch state {
	case Fresh:
		log.Printf("fresh: %s", req.URL)
		return cached.Response(), nil
	case Stale:
		if maxStale(cached) < delta {
			break
		}

		log.Printf("stale: %s", req.URL)
		return staleResponse(cached), nil
	case Revalidate:
		log.Printf("revalidate: %s", req.URL)
		req = revalidateRequest(req, cached)
	}

	reqTime := time.Now()
	resp, err := t.RoundTripper.RoundTrip(req)
	respTime := time.Now()
	if err != nil {
		if state == Stale {
			log.Printf("stale: %s", req.URL)
			return staleResponse(cached), nil
		}

		return resp, err
	}

	if revalidated(state, resp) {
		log.Printf("validated: %s", req.URL)
		resp = revalidatedResponse(resp, cached)
	}

	t.cacheIfPossible(req, resp, reqTime, respTime)

	return resp, nil
}

func (t *Transport) init() {
	if t.RoundTripper != nil {
		return
	}

	t.RoundTripper = http.DefaultTransport
}

func (t *Transport) roundTripClear(req *http.Request) (*http.Response, error) {
	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if successful(resp) {
		t.Clear()
	}

	return resp, nil
}

func idempotent(req *http.Request) bool {
	return req.Method == http.MethodGet || req.Method == http.MethodHead
}

func successful(resp *http.Response) bool {
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest
}

func revalidateRequest(orig *http.Request, cached *CachedResponse) *http.Request {
	req := &http.Request{
		Method: orig.Method,
		URL:    orig.URL,
		Header: http.Header{},
	}

	for k, v := range orig.Header {
		req.Header[k] = v
	}

	if etag := cached.Header.Get(etagField); etag != "" {
		req.Header.Set(ifNoneMatchField, etag)
	}

	if time := cached.Header.Get(lastModifiedField); time != "" {
		req.Header.Set(ifModifiedSinceField, time)
	}

	return req
}

func staleResponse(cached *CachedResponse) *http.Response {
	resp := cached.Response()
	resp.Header.Set(warningField, `110 - "Response is Stale"`)
	return resp
}

func revalidatedResponse(resp *http.Response, cached *CachedResponse) *http.Response {
	result := cached.Response()

	var warnings []string
	for _, warning := range values(result.Header, warningField) {
		if strings.HasPrefix(warning, "2") {
			warnings = append(warnings, warning)
		}
	}
	result.Header[warningField] = warnings

	for k, v := range resp.Header {
		if k == warningField {
			continue
		}
		result.Header[k] = v
	}

	return result
}

func revalidated(state CachedState, resp *http.Response) bool {
	return state == Revalidate && resp.StatusCode == http.StatusNotModified
}

func (t *Transport) cacheIfPossible(req *http.Request, resp *http.Response, reqTime, respTime time.Time) {
	if !Cacheable(req, resp) {
		return
	}

	cached, err := NewCachedResponse(resp, reqTime, respTime)
	if err != nil {
		log.Fatal(err)
	}
	t.Set(req, cached)
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
func State(req *http.Request, cached *CachedResponse) (CachedState, time.Duration) {
	if cached == nil {
		return Miss, time.Duration(0)
	}

	if contains(req.Header, pragmaField, noCache) {
		return Revalidate, time.Duration(0)
	}

	if contains(req.Header, cacheControlField, noCache) {
		return Revalidate, time.Duration(0)
	}

	if contains(cached.Header, cacheControlField, noCache) {
		return Revalidate, time.Duration(0)
	}

	if lifetime, ok := freshnessLifetime(cached); ok {
		age := currentAge(cached)
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

func freshnessLifetime(cached *CachedResponse) (time.Duration, bool) {
	if age, ok := sMaxage(cached); ok {
		return age, true
	}

	if age, ok := maxAge(cached); ok {
		return age, true
	}

	if t, ok := expires(cached); ok {
		return t.Sub(time.Now()), true
	}

	return 0, false
}

func sMaxage(cached *CachedResponse) (time.Duration, bool) {
	matches := matches(cached.Header, cacheControlField, sMaxagePattern)
	if matches == nil {
		return time.Duration(0), false
	}

	s, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Duration(s) * time.Second, false
	}

	return time.Duration(s) * time.Second, true
}

func maxAge(cached *CachedResponse) (time.Duration, bool) {
	matches := matches(cached.Header, cacheControlField, maxAgePattern)
	if matches == nil {
		return time.Duration(0), false
	}

	s, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Duration(s) * time.Second, false
	}

	return time.Duration(s) * time.Second, true
}

func expires(cached *CachedResponse) (time.Time, bool) {
	vs := cached.Header[expiresField]
	if len(vs) != 1 {
		return time.Now(), false
	}

	v := vs[0]

	// v has to be HTTP-time. https://www.w3.org/Protocols/rfc2616/rfc2616-sec3.html
	t, err := parseHTTPTime(v)
	if err != nil {
		return t, false
	}

	return t, true
}

func parseHTTPTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		return t, nil
	}

	if t, err := time.Parse(time.RFC850, s); err == nil {
		return t, nil
	}

	if t, err := time.Parse(time.ANSIC, s); err == nil {
		return t, nil
	}

	return time.Now(), fmt.Errorf("invalid HTTP time: %s", s)
}

func maxStale(cached *CachedResponse) time.Duration {
	matches := matches(cached.Header, cacheControlField, maxStalePattern)
	if matches == nil {
		return time.Duration(0)
	}

	s, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Duration(0)
	}

	return time.Duration(s) * time.Second
}

func currentAge(cached *CachedResponse) time.Duration {
	return correctedInitialAge(cached) + residentTime(cached)
}

func residentTime(cached *CachedResponse) time.Duration {
	return time.Since(cached.ResponseTime)
}

func correctedInitialAge(cached *CachedResponse) time.Duration {
	a := apparentAge(cached)
	c := correctedAgeValue(cached)

	if a < c {
		return c
	}

	return a
}

func apparentAge(cached *CachedResponse) time.Duration {
	date, ok := dateValue(cached)
	if !ok {
		return time.Duration(0)
	}

	a := cached.ResponseTime.Sub(date)

	if time.Duration(0) < a {
		return a
	}

	return time.Duration(0)
}

func dateValue(cached *CachedResponse) (time.Time, bool) {
	vs := values(cached.Header, dateField)
	if len(vs) != 1 {
		return time.Now(), false
	}

	t, err := time.Parse(time.RFC1123, vs[0])
	if err != nil {
		return time.Now(), false
	}

	return t, true
}

func correctedAgeValue(cached *CachedResponse) time.Duration {
	return ageValue(cached) + responseDelay(cached)
}

func ageValue(cached *CachedResponse) time.Duration {
	vs := values(cached.Header, ageField)
	if len(vs) != 1 {
		return time.Duration(0)
	}

	v := vs[0]

	s, err := strconv.Atoi(v)
	if err != nil {
		return time.Duration(0)
	}

	return time.Duration(s) * time.Second
}

func responseDelay(cached *CachedResponse) time.Duration {
	return cached.ResponseTime.Sub(cached.RequestTime)
}

// Cacheable checks if the req/resp pair is cacheable based on https://tools.ietf.org/html/rfc7234#section-3
func Cacheable(req *http.Request, resp *http.Response) bool {
	if req.Method != http.MethodGet {
		return false
	}

	if resp.StatusCode != http.StatusOK {
		return false
	}

	if contains(req.Header, cacheControlField, noCache) {
		return false
	}

	if contains(resp.Header, cacheControlField, noCacheOrPrivate) {
		return false
	}

	if _, ok := req.Header[authorizationField]; ok {
		if !contains(resp.Header, cacheControlField, mustRevalidateOrPublicOrSMaxage) {
			return false
		}
	}

	if _, ok := resp.Header[expiresField]; ok {
		return true
	}

	if contains(resp.Header, cacheControlField, maxAgeOrSMaxageOrPublic) {
		return true
	}

	return false
}

func contains(h http.Header, key string, value *regexp.Regexp) bool {
	return matches(h, key, value) != nil
}

func matches(h http.Header, key string, value *regexp.Regexp) []string {
	for _, v := range values(h, key) {
		result := value.FindStringSubmatch(v)

		if result != nil {
			return result
		}
	}

	return nil
}

func values(h http.Header, key string) []string {
	var result []string

	vs, ok := h[key]
	if !ok {
		return nil
	}

	for _, v := range vs {
		vs := strings.Split(v, ",")

		for _, v := range vs {
			result = append(result, strings.Trim(v, " "))
		}
	}

	return result
}
