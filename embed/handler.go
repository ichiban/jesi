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
	"github.com/ichiban/jesi/transaction"
	log "github.com/sirupsen/logrus"
	"strconv"
)

const (
	about    = "about"
	with     = "with"
	href     = "href"
	links    = "_links"
	embedded = "_embedded"

	cacheControlField  = "Cache-Control"
	contentTypeField   = "Content-Type"
	contentLengthField = "Content-Length"

	warningField = "Warning"
	etagField    = "Etag"
	expiresField = "Expires"
	withField    = "With"
)

var jsonPattern = regexp.MustCompile(`\Aapplication/(?:.+\+)?json`)

// Handler is an embedding handler.
type Handler struct {
	Next http.Handler
}

var _ http.Handler = (*Handler)(nil)

// ServeHTTP fetches a response from the underlying handler and if it contains links matching the embedding spec,
// also fetches linked documents and embeds them.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	spec := stripSpec(r)

	rep := cache.NewRepresentation(h.Next, r)
	defer func() {
		rep.HeaderMap.Set(contentLengthField, strconv.Itoa(len(rep.Body)))
		if _, err := rep.WriteTo(w); err != nil {
			log.WithFields(log.Fields{
				"id":    transaction.ID(r),
				"error": err,
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

	doc := &document{
		CacheControl: NewCacheControl(rep),
		data:         data,
	}
	h.embed(r, doc, spec)

	delete(rep.HeaderMap, expiresField)
	rep.HeaderMap[cacheControlField] = []string{doc.CacheControl.String()}

	var err error
	rep.Body, err = json.Marshal(doc.data)
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
		"id": transaction.ID(r),
	}).Debug("Finished a request")
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

	for _, ws := range req.Header[withField] {
		for _, w := range strings.Split(ws, ",") {
			w = strings.TrimSpace(w)
			if strings.HasPrefix(w, `"`) {
				w = w[1 : len(w)-1]
			}
			spec.add(strings.Split(w, "."))
		}
	}

	return spec
}

type document struct {
	*CacheControl
	edge string
	pos  *int
	data interface{}
}

func (h *Handler) embed(base *http.Request, doc *document, spec specifier) {
	if len(spec) == 0 {
		return
	}

	parent := doc.data.(map[string]interface{})
	ls := parent[links].(map[string]interface{})
	es, ok := parent[embedded].(map[string]interface{})
	if !ok {
		m := make(map[string]interface{})
		parent[embedded] = m
		es = m
	}

	ch := make(chan *document, len(spec))
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
			go h.fetch(base, edge, nil, l[href].(string), next, ch)
		case []interface{}:
			es[edge] = make([]interface{}, len(l))
			for i, l := range l {
				i := i
				l := l.(map[string]interface{})
				count++
				go h.fetch(base, edge, &i, l[href].(string), next, ch)
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
		doc.CacheControl = doc.CacheControl.Merge(sub.CacheControl)
	}
}

func (h *Handler) fetch(base *http.Request, edge string, pos *int, href string, next specifier, ch chan<- *document) {
	uri, err := url.Parse(href)
	if err != nil {
		ch <- errorDocument(edge, pos, NewMalformedURLError(err))
		return
	}

	log.WithFields(log.Fields{
		"id":   transaction.ID(base),
		"edge": edge,
		"pos":  pos,
		"href": uri,
		"next": next,
	}).Debug("Will fetch a subdocument")

	req, err := http.NewRequest(http.MethodGet, uri.String(), nil)
	if err != nil {
		ch <- errorDocument(edge, pos, NewMalformedSubRequestError(err, uri))
		return
	}
	req = req.WithContext(base.Context())
	for k, vs := range base.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	rep := cache.NewRepresentation(h.Next, req)
	if !rep.Successful() {
		ch <- errorDocument(edge, pos, NewResponseError(rep, uri))
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(rep.Body, &data); err != nil {
		ch <- errorDocument(edge, pos, NewMalformedJSONError(err, uri))
		return
	}

	doc := &document{
		CacheControl: NewCacheControl(rep),
		data:         data,
	}
	h.embed(base, doc, next)

	ch <- &document{
		CacheControl: doc.CacheControl,
		edge:         edge,
		pos:          pos,
		data:         doc.data,
	}

	log.WithFields(log.Fields{
		"child":  transaction.ID(req),
		"parent": transaction.ID(base),
	}).Debug("Finished a subrequest")
}

func errorDocument(edge string, pos *int, e *Error) *document {
	return &document{
		CacheControl: &CacheControl{
			NoStore: true,
		},
		edge: edge,
		pos:  pos,
		data: e,
	}
}
