package embed

import (
	"bytes"
	"crypto/md5" // #nosec
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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

// Transport is an embedding transport.
type Transport struct {
	http.RoundTripper
}

var _ http.RoundTripper = (*Transport)(nil)

// RoundTrip fetches a response from the underlying transport and if it contains links matching the embedding spec,
// also fetches linked documents and embeds them.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.RoundTripper
	if base == nil {
		base = http.DefaultTransport
	}

	if req.Method != http.MethodGet {
		return base.RoundTrip(req)
	}

	spec := stripSpec(req)

	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if !jsonPattern.MatchString(resp.Header.Get(contentTypeField)) {
		return resp, nil
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp, err
	}

	if err = resp.Body.Close(); err != nil {
		return resp, err
	}

	var data map[string]interface{}
	if err = json.Unmarshal(b, &data); err != nil {
		return resp, err
	}

	res := &resource{
		cacheControl: NewCacheControl(resp),
		data:         data,
	}
	t.embed(req, res, spec)

	delete(resp.Header, expires)
	resp.Header[cacheControl] = []string{res.cacheControl.String()}

	b, err = json.Marshal(res.data)
	if err != nil {
		return resp, err
	}

	h := md5.New() // #nosec
	_, _ = h.Write(b)
	etag := fmt.Sprintf(`W/"%s"`, hex.EncodeToString(h.Sum(nil)))
	resp.Header[etagField] = []string{etag}

	resp.Body = ioutil.NopCloser(bytes.NewReader(b))

	if _, ok := resp.Header[warningField]; !ok {
		resp.Header.Set(warningField, `214 - "Transformation Applied"`)
	}

	return resp, nil
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

func (t *Transport) embed(req *http.Request, res *resource, spec specifier) {
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
	count := 0
	for edge, next := range spec {
		l, ok := ls[edge]
		if !ok {
			continue
		}

		switch l := l.(type) {
		case map[string]interface{}:
			count++
			go t.fetch(req, edge, nil, l[href].(string), next, ch)
		case []interface{}:
			es[edge] = make([]interface{}, len(l))
			for i, l := range l {
				i := i
				l := l.(map[string]interface{})
				count++
				go t.fetch(req, edge, &i, l[href].(string), next, ch)
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

func (t *Transport) fetch(base *http.Request, edge string, pos *int, href string, next specifier, ch chan<- *resource) {
	transport := t.RoundTripper
	if transport == nil {
		transport = http.DefaultTransport
	}

	uri, err := url.Parse(href)
	if err != nil {
		ch <- errorResource(edge, pos, NewMalformedURLError(err))
	}

	req, err := http.NewRequest(http.MethodGet, base.URL.ResolveReference(uri).String(), nil)
	if err != nil {
		ch <- errorResource(edge, pos, NewMalformedSubRequestError(err, uri))
	}
	req.Header = base.Header

	resp, err := transport.RoundTrip(req)
	if err != nil {
		ch <- errorResource(edge, pos, NewRoundTripError(err, uri))
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		ch <- errorResource(edge, pos, NewResponseError(resp, uri))
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		ch <- errorResource(edge, pos, NewResponseBodyReadError(err, uri))
	}

	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		ch <- errorResource(edge, pos, NewMalformedJSONError(err, uri))
	}

	res := &resource{
		cacheControl: NewCacheControl(resp),
		data:         data,
	}
	t.embed(req, res, next)

	ch <- &resource{
		edge:         edge,
		pos:          pos,
		cacheControl: res.cacheControl,
		data:         res.data,
	}
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
