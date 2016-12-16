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

type Transport struct {
	http.RoundTripper
	Cache
}

var _ http.RoundTripper = (*Transport)(nil)

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.RoundTripper == nil {
		t.RoundTripper = http.DefaultTransport
	}

	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		resp, err := t.RoundTripper.RoundTrip(req)
		if err != nil {
			return resp, err
		}

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
			t.Clear()
		}

		return resp, nil
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
		if maxStale, ok := maxStale(cached); ok && maxStale > delta {
			log.Printf("stale: %s", req.URL)
			resp := cached.Response()
			resp.Header.Set(warningField, `110 - "Response is Stale"`)
			return resp, nil
		}
	case Revalidate:
		log.Printf("revalidate: %s", req.URL)

		orig := req

		req = &http.Request{
			Method: req.Method,
			URL:    req.URL,
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
	}

	log.Printf("from backend: %s", req.URL)

	reqTime := time.Now()
	resp, err := t.RoundTripper.RoundTrip(req)
	respTime := time.Now()
	if err != nil {
		if state == Stale {
			log.Printf("stale: %s", req.URL)
			resp := cached.Response()
			resp.Header.Set(warningField, `110 - "Response is Stale"`)
			return resp, nil
		}

		return resp, err
	}

	if state == Revalidate && resp.StatusCode == http.StatusNotModified {
		log.Printf("validated: %s", req.URL)

		h := resp.Header
		resp = cached.Response()

		var warnings []string
		for _, warning := range values(resp.Header, warningField) {
			if strings.HasPrefix(warning, "2") {
				warnings = append(warnings, warning)
			}
		}
		resp.Header[warningField] = warnings

		for k, v := range h {
			if k == warningField {
				continue
			}
			resp.Header[k] = v
		}
	}

	if Cacheable(req, resp) {
		cached, err := NewCachedResponse(resp, reqTime, respTime)
		if err != nil {
			log.Fatal(err)
		}
		t.Set(req, cached)
	}

	if _, ok := resp.Header[warningField]; !ok {
		resp.Header.Set(warningField, `214 - "Transformation Applied"`)
	}

	return resp, nil
}

type CacheState int

const (
	Miss CacheState = iota
	Fresh
	Stale
	Revalidate
)

// State returns the state of cached response.
func State(req *http.Request, cached *CachedResponse) (CacheState, time.Duration) {
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

func maxStale(cached *CachedResponse) (time.Duration, bool) {
	matches := matches(cached.Header, cacheControlField, maxStalePattern)
	if matches == nil {
		return time.Duration(0), false
	}

	s, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Duration(s) * time.Second, false
	}

	return time.Duration(s) * time.Second, true
}

func currentAge(cached *CachedResponse) time.Duration {
	return correctedInitialAge(cached) + residentTime(cached)
}

func residentTime(cached *CachedResponse) time.Duration {
	return time.Now().Sub(cached.ResponseTime)
}

func correctedInitialAge(cached *CachedResponse) time.Duration {
	a := apparentAge(cached)
	c := correctedAgeValue(cached)

	if a < c {
		return c
	} else {
		return a
	}
}

func apparentAge(cached *CachedResponse) time.Duration {
	date, ok := dateValue(cached)
	if !ok {
		return time.Duration(0)
	}

	a := cached.ResponseTime.Sub(date)

	if time.Duration(0) < a {
		return a
	} else {
		return time.Duration(0)
	}
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
