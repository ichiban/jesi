package cache

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ichiban/jesi/transaction"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
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
	*Store
}

var _ http.Handler = (*Handler)(nil)

// ServeHTTP returns a cached response if found. Otherwise, retrieves one from the underlying handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cached := h.Get(r)
	state, delta := h.State(r, cached)

	log.WithFields(log.Fields{
		"id":    transaction.ID(r),
		"state": state,
		"delta": delta,
	}).Debug("Got a state of a representation")

	switch state {
	case Fresh:
		serveFresh(w, cached, r)
		return
	case Stale:
		if max := maxStale(cached); max < delta {
			log.WithFields(log.Fields{
				"id":        transaction.ID(r),
				"max-stale": max,
				"delta":     delta,
			}).Debug("Delta exceeded max-stale.")

			break
		}

		serveStale(w, cached, r)
		return
	case Revalidate:
		r = revalidateRequest(r, cached)
	}

	// Keep the original request since `balance.Handler` will modify the request.
	origReq := *r
	origURL := *r.URL
	origReq.URL = &origURL

	var rep Representation
	rep.ID = uuid.NewV4()
	rep.RequestTime = time.Now()
	h.Next.ServeHTTP(&rep, r)
	rep.ResponseTime = time.Now()
	defer func() {
		if _, err := rep.WriteTo(w); err != nil {
			log.WithFields(log.Fields{
				"id":    transaction.ID(r),
				"error": err,
			}).Error("Couldn't write a response")
		}
	}()

	if !rep.Successful() {
		log.WithFields(log.Fields{
			"id":     transaction.ID(r),
			"status": rep.StatusCode,
		}).Debug("Couldn't get a successful response")

		if state == Stale {
			log.WithFields(log.Fields{
				"id": transaction.ID(r),
			}).Debug("Will serve a stale response")

			rep = *staleResponse(cached)
		}
		return
	}

	if originChanged(r, &rep) {
		h.OriginChangedAt = rep.ResponseTime

		log.WithFields(log.Fields{
			"id": transaction.ID(r),
			"at": rep.ResponseTime,
		}).Debug("The origin changed")
	}

	if revalidated(state, &rep) {
		log.WithFields(log.Fields{
			"id": transaction.ID(r),
		}).Debug("Will serve a revalidated response")

		rep = *revalidatedResponse(&rep, cached)
	}

	h.cacheIfPossible(&origReq, &rep)
}

func serveFresh(w io.Writer, cached *Representation, r *http.Request) {
	if _, err := cached.WriteTo(w); err != nil {
		log.WithFields(log.Fields{
			"id":    transaction.ID(r),
			"error": err,
		}).Error("Couldn't write a response")
	}
}

func serveStale(w io.Writer, cached *Representation, r *http.Request) {
	resp := staleResponse(cached)
	if _, err := resp.WriteTo(w); err != nil {
		log.WithFields(log.Fields{
			"id":    transaction.ID(r),
			"error": err,
		}).Error("Couldn't write a response")
	}
}

func originChanged(req *http.Request, rep *Representation) bool {
	return !idempotent(req) && successful(rep)
}

func idempotent(req *http.Request) bool {
	return req.Method == http.MethodGet || req.Method == http.MethodHead
}

func successful(rep *Representation) bool {
	return rep.StatusCode >= http.StatusOK && rep.StatusCode < http.StatusBadRequest
}

func revalidateRequest(orig *http.Request, cached *Representation) *http.Request {
	req := &http.Request{
		Method: orig.Method,
		URL:    orig.URL,
		Header: http.Header{},
	}

	for k, v := range orig.Header {
		req.Header[k] = v
	}

	if etag := cached.HeaderMap.Get(etagField); etag != "" {
		req.Header.Set(ifNoneMatchField, etag)
	}

	if time := cached.HeaderMap.Get(lastModifiedField); time != "" {
		req.Header.Set(ifModifiedSinceField, time)
	}

	return req
}

func staleResponse(cached *Representation) *Representation {
	cached.HeaderMap.Set(warningField, `110 - "Response is Stale"`)
	return cached
}

func revalidatedResponse(rep *Representation, cached *Representation) *Representation {
	var warnings []string
	for _, warning := range values(cached.HeaderMap, warningField) {
		if strings.HasPrefix(warning, "2") {
			warnings = append(warnings, warning)
		}
	}
	cached.HeaderMap[warningField] = warnings

	for k, v := range rep.HeaderMap {
		if k == warningField {
			continue
		}
		cached.HeaderMap[k] = v
	}

	return cached
}

func revalidated(state CachedState, rep *Representation) bool {
	return state == Revalidate && rep.StatusCode == http.StatusNotModified
}

func (h *Handler) cacheIfPossible(req *http.Request, rep *Representation) {
	if !Cacheable(req, rep) {
		return
	}
	h.Set(req, rep)
}

func freshnessLifetime(cached *Representation) (time.Duration, bool) {
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

func sMaxage(cached *Representation) (time.Duration, bool) {
	matches := matches(cached.HeaderMap, cacheControlField, sMaxagePattern)
	if matches == nil {
		return time.Duration(0), false
	}

	s, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Duration(s) * time.Second, false
	}

	return time.Duration(s) * time.Second, true
}

func maxAge(cached *Representation) (time.Duration, bool) {
	matches := matches(cached.HeaderMap, cacheControlField, maxAgePattern)
	if matches == nil {
		return time.Duration(0), false
	}

	s, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Duration(s) * time.Second, false
	}

	return time.Duration(s) * time.Second, true
}

func expires(cached *Representation) (time.Time, bool) {
	vs := cached.HeaderMap[expiresField]
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

func maxStale(cached *Representation) time.Duration {
	matches := matches(cached.HeaderMap, cacheControlField, maxStalePattern)
	if matches == nil {
		return time.Duration(0)
	}

	s, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Duration(0)
	}

	return time.Duration(s) * time.Second
}

func currentAge(cached *Representation) time.Duration {
	return correctedInitialAge(cached) + residentTime(cached)
}

func residentTime(cached *Representation) time.Duration {
	return time.Since(cached.ResponseTime)
}

func correctedInitialAge(cached *Representation) time.Duration {
	a := apparentAge(cached)
	c := correctedAgeValue(cached)

	if a < c {
		return c
	}

	return a
}

func apparentAge(cached *Representation) time.Duration {
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

func dateValue(cached *Representation) (time.Time, bool) {
	vs := values(cached.HeaderMap, dateField)
	if len(vs) != 1 {
		return time.Now(), false
	}

	t, err := time.Parse(time.RFC1123, vs[0])
	if err != nil {
		return time.Now(), false
	}

	return t, true
}

func correctedAgeValue(cached *Representation) time.Duration {
	return ageValue(cached) + responseDelay(cached)
}

func ageValue(cached *Representation) time.Duration {
	vs := values(cached.HeaderMap, ageField)
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

func responseDelay(cached *Representation) time.Duration {
	return cached.ResponseTime.Sub(cached.RequestTime)
}

// Cacheable checks if the req/resp pair is cacheable based on https://tools.ietf.org/html/rfc7234#section-3
func Cacheable(req *http.Request, rep *Representation) bool {
	if req.Method != http.MethodGet {
		return false
	}

	if rep.StatusCode != http.StatusOK {
		return false
	}

	if contains(req.Header, cacheControlField, noStore) {
		return false
	}

	if contains(rep.HeaderMap, cacheControlField, noStoreOrPrivate) {
		return false
	}

	if _, ok := req.Header[authorizationField]; ok {
		if !contains(rep.HeaderMap, cacheControlField, mustRevalidateOrPublicOrSMaxage) {
			return false
		}
	}

	if _, ok := rep.HeaderMap[expiresField]; ok {
		return true
	}

	if contains(rep.HeaderMap, cacheControlField, maxAgeOrSMaxageOrPublic) {
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
