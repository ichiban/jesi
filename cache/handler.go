package cache

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ichiban/jesi/common"
	log "github.com/sirupsen/logrus"
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
	noStore                         = regexp.MustCompile(`\Ano-store\z`)
	noStoreOrPrivate                = regexp.MustCompile(`\A(?:no-store|private)\z`)
	mustRevalidateOrPublicOrSMaxage = regexp.MustCompile(`\A(?:must-revalidate|public|s-maxage=\d+)\z`)
	maxAgeOrSMaxageOrPublic         = regexp.MustCompile(`\A(?:max-age=\d+|s-maxage=\d+|public)\z`)

	sMaxagePattern = regexp.MustCompile(`\As-maxage=(\d+)\z`)
	maxAgePattern  = regexp.MustCompile(`\Amax-age=(\d+)\z`)

	revalidatePattern = regexp.MustCompile(`\A(?:s-maxage=\d+|(?:must|proxy)-revalidate)\z`)

	maxStalePattern = regexp.MustCompile(`\Amax-stale=(\d+)\z`)
)

// Handler is a caching handler.
type Handler struct {
	Next http.Handler
	Cache
}

var _ http.Handler = (*Handler)(nil)

// ServeHTTP returns a cached response if found. Otherwise, retrieves one from the underlying handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.WithFields(log.Fields{
		"request": r.URL,
	}).Debug("Will serve a cached response if exists")

	cached := h.Get(r)
	state, delta := h.State(r, cached)
	log.WithFields(log.Fields{
		"request": r.URL,
		"state":   state,
		"delta":   delta,
	}).Info("Got a state of a request")

	switch state {
	case Fresh:
		serveFresh(w, cached)
		return
	case Stale:
		if max := maxStale(cached); max < delta {
			log.WithFields(log.Fields{
				"max-stale": max,
				"delta":     delta,
			}).Debug("Delta exceeded max-stale.")

			break
		}

		serveStale(w, cached)
		return
	case Revalidate:
		r = revalidateRequest(r, cached)
	}

	// Keep the original request since `balance.Handler` will modify the request.
	origReq := *r
	origURL := *r.URL
	origReq.URL = &origURL

	var resp common.ResponseBuffer
	reqTime := time.Now()
	h.Next.ServeHTTP(&resp, r)
	respTime := time.Now()
	defer func() {
		if _, err := resp.WriteTo(w); err != nil {
			log.WithFields(log.Fields{
				"request": r.URL,
				"error":   err,
			}).Error("Couldn't write a response")
		}
	}()

	if !resp.Successful() {
		log.WithFields(log.Fields{
			"request": r.URL,
			"status":  resp.StatusCode,
		}).Debug("Couldn't get a successful response")

		if state == Stale {
			log.WithFields(log.Fields{
				"request": r.URL,
			}).Debug("Will serve a stale response")

			resp = *staleResponse(cached)
		}
		return
	}

	if originChanged(r, &resp) {
		h.OriginChangedAt = respTime

		log.WithFields(log.Fields{
			"at": respTime,
		}).Info("The origin changed")
	}

	if revalidated(state, &resp) {
		log.WithFields(log.Fields{
			"request": r.URL,
		}).Debug("Will serve a revalidated response")

		resp = *revalidatedResponse(&resp, cached)
	}

	h.cacheIfPossible(&origReq, &resp, reqTime, respTime)
}

func serveFresh(w io.Writer, cached *CachedResponse) {
	resp := cached.Response()
	if _, err := resp.WriteTo(w); err != nil {
		log.Print(err)
	}
}

func serveStale(w io.Writer, cached *CachedResponse) {
	resp := staleResponse(cached)
	if _, err := resp.WriteTo(w); err != nil {
		log.Print(err)
	}
}

func originChanged(req *http.Request, resp *common.ResponseBuffer) bool {
	return !idempotent(req) && successful(resp)
}

func idempotent(req *http.Request) bool {
	return req.Method == http.MethodGet || req.Method == http.MethodHead
}

func successful(resp *common.ResponseBuffer) bool {
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

func staleResponse(cached *CachedResponse) *common.ResponseBuffer {
	resp := cached.Response()
	resp.HeaderMap.Set(warningField, `110 - "Response is Stale"`)
	return resp
}

func revalidatedResponse(resp *common.ResponseBuffer, cached *CachedResponse) *common.ResponseBuffer {
	result := cached.Response()

	var warnings []string
	for _, warning := range values(result.HeaderMap, warningField) {
		if strings.HasPrefix(warning, "2") {
			warnings = append(warnings, warning)
		}
	}
	result.HeaderMap[warningField] = warnings

	for k, v := range resp.HeaderMap {
		if k == warningField {
			continue
		}
		result.HeaderMap[k] = v
	}

	return result
}

func revalidated(state CachedState, resp *common.ResponseBuffer) bool {
	return state == Revalidate && resp.StatusCode == http.StatusNotModified
}

func (h *Handler) cacheIfPossible(req *http.Request, resp *common.ResponseBuffer, reqTime, respTime time.Time) {
	if !Cacheable(req, resp) {
		return
	}

	cached, err := NewCachedResponse(resp, reqTime, respTime)
	if err != nil {
		log.Fatal(err)
	}
	h.Set(req, cached)
}

func freshnessLifetime(cached *CachedResponse) (time.Duration, bool) {
	if age, ok := sMaxage(cached); ok {
		return age, true
	}

	if age, ok := maxAge(cached); ok {
		return age, true
	}

	if t, ok := expires(cached); ok {
		return time.Until(t), true
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
func Cacheable(req *http.Request, resp *common.ResponseBuffer) bool {
	if req.Method != http.MethodGet {
		return false
	}

	if resp.StatusCode != http.StatusOK {
		return false
	}

	if contains(req.Header, cacheControlField, noStore) {
		return false
	}

	if contains(resp.HeaderMap, cacheControlField, noStoreOrPrivate) {
		return false
	}

	if _, ok := req.Header[authorizationField]; ok {
		if !contains(resp.HeaderMap, cacheControlField, mustRevalidateOrPublicOrSMaxage) {
			return false
		}
	}

	if _, ok := resp.HeaderMap[expiresField]; ok {
		return true
	}

	if contains(resp.HeaderMap, cacheControlField, maxAgeOrSMaxageOrPublic) {
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
