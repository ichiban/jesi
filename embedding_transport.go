package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	with = "with"
	href = "href"
	links = "_links"
	embedded = "_embedded"
)

type EmbeddingTransport struct {
	http.RoundTripper
}

func (e *EmbeddingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	log.Printf("uri: %s", req.URL.String())

	base := e.RoundTripper
	if base == nil {
		base = http.DefaultTransport
	}

	if req.Method != http.MethodGet {
		return base.RoundTrip(req)
	}

	spec, err := stripSpec(req)
	if err != nil {
		return nil, err
	}

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
		log.Fatalf("json.Unmarshal() failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go e.embed(req, &wg, data, spec)
	wg.Wait()

	b, err = json.Marshal(data)
	if err != nil {
		return resp, err
	}

	resp.Body = ioutil.NopCloser(bytes.NewReader(b))

	return resp, nil
}

func stripSpec(req *http.Request) ([]string, error) {
	w := req.URL.Query().Get(with)
	spec := strings.Split(w, ".")

	q := req.URL.Query()
	q.Del(with)
	req.URL.RawQuery = q.Encode()

	return spec, nil
}

func (e *EmbeddingTransport) embed(req *http.Request, wg *sync.WaitGroup, parent map[string]interface{}, spec []string) {
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
		child, err := e.get(req, l)
		if err != nil {
			log.Fatal(err)
			return
		}
		es[spec[0]] = child

		wg.Add(1)
		go e.embed(req, wg, child, spec[1:])
	case []interface{}:
		children := make([]map[string]interface{}, len(l))
		var cwg sync.WaitGroup
		for i, m := range l {
			cwg.Add(1)
			go func(i int, link map[string]interface{}) {
				defer cwg.Done()

				child, err := e.get(req, link)
				if err != nil {
					log.Fatal(err)
					return
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

func (e *EmbeddingTransport) get(base *http.Request, link map[string]interface{}) (map[string]interface{}, error) {
	transport := e.RoundTripper
	if transport == nil {
		transport = http.DefaultTransport
	}

	h, ok := link[href].(string)
	if !ok {
		return nil, errors.New("href not found")
	}

	uri, err := url.Parse(h)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, base.URL.ResolveReference(uri).String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header = base.Header

	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}

	return data, nil
}
