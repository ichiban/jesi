package embed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	about    = "about"
	with     = "with"
	href     = "href"
	links    = "_links"
	embedded = "_embedded"
	errs     = "errors"
)

type Transport struct {
	http.RoundTripper
}

var _ http.RoundTripper = (*Transport)(nil)

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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp, err

	}

	if err := resp.Body.Close(); err != nil {
		return resp, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return resp, err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go t.embed(req, &wg, data, spec)
	wg.Wait()

	b, err = json.Marshal(data)
	if err != nil {
		return resp, err
	}

	resp.Body = ioutil.NopCloser(bytes.NewReader(b))

	return resp, nil
}

func stripSpec(req *http.Request) []string {
	w := req.URL.Query().Get(with)
	spec := strings.Split(w, ".")

	q := req.URL.Query()
	q.Del(with)
	req.URL.RawQuery = q.Encode()

	return spec
}

func (e *Transport) embed(req *http.Request, wg *sync.WaitGroup, parent map[string]interface{}, spec []string) {
	defer wg.Done()

	if len(spec) == 0 {
		return
	}

	ls, ok := parent[links].(map[string]interface{})
	if !ok {
		return
	}

	l, ok := ls[spec[0]]
	if !ok {
		return
	}

	es, ok := parent[embedded].(map[string]interface{})
	if !ok {
		m := make(map[string]interface{})
		parent[embedded] = m
		es = m
	}

	switch l := l.(type) {
	case map[string]interface{}:
		child, err := e.fetch(req, l)
		if err, ok := err.(*Error); ok {
			es[errs] = []*Error{err}
			return
		}
		if err != nil {
			log.Fatal(err)
		}
		es[spec[0]] = child

		wg.Add(1)
		go e.embed(req, wg, child, spec[1:])
	case []interface{}:
		var errMu sync.Mutex

		children := make([]map[string]interface{}, len(l))
		var cwg sync.WaitGroup
		for i, m := range l {
			cwg.Add(1)
			go func(i int, link map[string]interface{}) {
				defer cwg.Done()

				child, err := e.fetch(req, link)
				if err, ok := err.(*Error); ok {
					errMu.Lock()
					if _, ok := es[errs]; !ok {
						var m []*Error
						es[errs] = m
					}
					es[errs] = append(es[errs].([]*Error), err)
					errMu.Unlock()
					return
				}
				if err != nil {
					log.Fatal(err)
				}
				children[i] = child

				wg.Add(1)
				go e.embed(req, wg, child, spec[1:])
			}(i, m.(map[string]interface{}))
		}
		cwg.Wait()
		es[spec[0]] = children
	}
}

func (e *Transport) fetch(base *http.Request, link map[string]interface{}) (map[string]interface{}, error) {
	transport := e.RoundTripper
	if transport == nil {
		transport = http.DefaultTransport
	}

	h, ok := link[href].(string)
	if !ok {
		return nil, &Error{
			Title:  "Malformed Link",
			Detail: fmt.Sprintf("href not found in %v", link),
		}
	}

	uri, err := url.Parse(h)
	if err != nil {
		return nil, &Error{
			Title:  "Malformed URL",
			Detail: err.Error(),
			Links: map[string]interface{}{
				about: h,
			},
		}
	}

	req, err := http.NewRequest(http.MethodGet, base.URL.ResolveReference(uri).String(), nil)
	if err != nil {
		return nil, &Error{
			Title:  "Malformed Subrequest",
			Detail: err.Error(),
			Links: map[string]interface{}{
				about: h,
			},
		}
	}
	req.Header = base.Header

	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nil, &Error{
			Title:  "Round Trip Error",
			Detail: err.Error(),
			Links: map[string]interface{}{
				about: h,
			},
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, &Error{
			Status: resp.StatusCode,
			Title:  "Error Response",
			Detail: http.StatusText(resp.StatusCode),
			Links: map[string]interface{}{
				about: h,
			},
		}
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, &Error{
			Title:  "Read Error",
			Detail: err.Error(),
			Links: map[string]interface{}{
				about: h,
			},
		}
	}

	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, &Error{
			Title:  "Malformed JSON",
			Detail: err.Error(),
			Links: map[string]interface{}{
				about: h,
			},
		}
	}

	return data, nil
}
