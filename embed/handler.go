package embed

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/ichiban/jesi/cache"
	"github.com/ichiban/jesi/request"
	log "github.com/sirupsen/logrus"
)

const (
	about    = "about"
	with     = "with"
	href     = "href"
	links    = "_links"
	embedded = "_embedded"

	contentTypeField = "Content-Type"
	warningField     = "Warning"
	etagField        = "Etag"
)

var jsonPattern = regexp.MustCompile(`\Aapplication/(?:json|hal\+json)`)

// Handler is an embedding handler.
type Handler struct {
	Next http.Handler
}

var _ http.Handler = (*Handler)(nil)

// ServeHTTP fetches a response from the underlying handler and if it contains links matching the embedding spec,
// also fetches linked documents and embeds them.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = request.WithID(r)
	log.WithFields(log.Fields{
		"request": request.ID(r),
	}).Info("Started a request")

	spec := stripSpec(r)

	var rep cache.Representation
	h.Next.ServeHTTP(&rep, r)
	defer func() {
		if _, err := rep.WriteTo(w); err != nil {
			log.WithFields(log.Fields{
				"request": request.ID(r),
				"error":   err,
			}).Error("Couldn't write a response")
		}
	}()

	if !jsonPattern.MatchString(rep.HeaderMap.Get(contentTypeField)) {
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(rep.Body, &data); err != nil {
		return
	}

	res := &resource{
		cacheControl: NewCacheControl(&rep),
		data:         data,
	}
	h.embed(r, res, spec)

	delete(rep.HeaderMap, expires)
	rep.HeaderMap[cacheControl] = []string{res.cacheControl.String()}

	var err error
	rep.Body, err = json.Marshal(res.data)
	if err != nil {
		return
	}

	ha := md5.New() // #nosec
	_, _ = ha.Write(rep.Body)
	etag := fmt.Sprintf(`W/"%s"`, hex.EncodeToString(ha.Sum(nil)))
	rep.HeaderMap[etagField] = []string{etag}

	if _, ok := rep.HeaderMap[warningField]; !ok {
		rep.HeaderMap.Set(warningField, `214 - "Transformation Applied"`)
	}

	log.WithFields(log.Fields{
		"request": request.ID(r),
	}).Info("Finished a request")
}

type specifier map[string]specifier

func (s specifier) add(edges []string) {
	n := s
	for _, edge := range edges {
		if _, ok := n[edge]; !ok {
			n[edge] = make(map[string]specifier)
		}
		n = n[edge]
	}
}

func stripSpec(req *http.Request) specifier {
	spec := specifier{}
	for _, w := range req.URL.Query()[with] {
		spec.add(strings.Split(w, "."))
	}

	q := req.URL.Query()
	q.Del(with)
	req.URL.RawQuery = q.Encode()

	return spec
}

type resource struct {
	edge         string
	pos          *int
	cacheControl *CacheControl
	data         interface{}
}

func (h *Handler) embed(req *http.Request, res *resource, spec specifier) {
	if len(spec) == 0 {
		return
	}

	parent := res.data.(map[string]interface{})
	ls := parent[links].(map[string]interface{})
	es, ok := parent[embedded].(map[string]interface{})
	if !ok {
		m := make(map[string]interface{})
		parent[embedded] = m
		es = m
	}

	ch := make(chan *resource, len(spec))
	defer close(ch)

	count := 0
	for edge, next := range spec {
		l, ok := ls[edge]
		if !ok {
			continue
		}

		switch l := l.(type) {
		case map[string]interface{}:
			count++
			go h.fetch(req, edge, nil, l[href].(string), next, ch)
		case []interface{}:
			es[edge] = make([]interface{}, len(l))
			for i, l := range l {
				i := i
				l := l.(map[string]interface{})
				count++
				go h.fetch(req, edge, &i, l[href].(string), next, ch)
			}
		}
	}

	for i := 0; i < count; i++ {
		sub := <-ch
		if sub.pos == nil {
			es[sub.edge] = sub.data
		} else {
			es[sub.edge].([]interface{})[*sub.pos] = sub.data
		}
		res.cacheControl = res.cacheControl.Merge(sub.cacheControl)
	}
}

func (h *Handler) fetch(base *http.Request, edge string, pos *int, href string, next specifier, ch chan<- *resource) {
	uri, err := url.Parse(href)
	if err != nil {
		ch <- errorResource(edge, pos, NewMalformedURLError(err))
		return
	}

	log.WithFields(log.Fields{
		"request": request.ID(base),
		"edge":    edge,
		"pos":     pos,
		"href":    uri,
		"next":    next,
	}).Debug("Will fetch a subresource")

	req, err := http.NewRequest(http.MethodGet, uri.String(), nil)
	if err != nil {
		ch <- errorResource(edge, pos, NewMalformedSubRequestError(err, uri))
		return
	}
	req.Header = base.Header
	req = request.WithID(req)
	log.WithFields(log.Fields{
		"request": request.ID(req),
		"parent":  request.ID(base),
	}).Info("Started a subrequest")

	var resp cache.Representation
	h.Next.ServeHTTP(&resp, req)

	if !resp.Successful() {
		ch <- errorResource(edge, pos, NewResponseError(&resp, uri))
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		ch <- errorResource(edge, pos, NewMalformedJSONError(err, uri))
		return
	}

	res := &resource{
		cacheControl: NewCacheControl(&resp),
		data:         data,
	}
	h.embed(req, res, next)

	ch <- &resource{
		edge:         edge,
		pos:          pos,
		cacheControl: res.cacheControl,
		data:         res.data,
	}

	log.WithFields(log.Fields{
		"request": request.ID(req),
		"parent":  request.ID(base),
	}).Info("Finished a subrequest")
}

func errorResource(edge string, pos *int, e *Error) *resource {
	return &resource{
		edge: edge,
		pos:  pos,
		cacheControl: &CacheControl{
			NoCache: true,
		},
		data: e,
	}
}
