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
)

type EmbedTransport struct {
	http.RoundTripper
}

func (e *EmbedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	log.Printf("uri: %s", req.URL.String())

	base := e.RoundTripper
	if base == nil {
		base = http.DefaultTransport
	}

	if req.Method != http.MethodGet {
		return base.RoundTrip(req)
	}

	r, spec, err := stripSpec(req)
	if err != nil {
		return nil, err
	}

	resp, err := base.RoundTrip(r)
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

	links, ok := data["_links"].(map[string]interface{})
	if !ok {
		return resp, nil
	}

	embedded, ok := data["_embedded"].(map[string]interface{})
	if !ok {
		m := make(map[string]interface{})
		data["_embedded"] = m
		embedded = m
	}

	current := []map[string]interface{}{data}
	for i := 0; i < len(spec); i++ {
		for _, c := range current {
			links, ok = c["_links"].(map[string]interface{})
			if !ok {
				break
			}

			link, ok := links[spec[i]]
			if !ok {
				break
			}

			embedded, ok = c["_embedded"].(map[string]interface{})
			if !ok {
				m := make(map[string]interface{})
				c["_embedded"] = m
				embedded = m
			}

			switch link := link.(type) {
			case map[string]interface{}:
				embed, err := e.get(req, link)
				if err != nil {
					log.Fatal(err)
					continue
				}

				embedded[spec[i]] = embed

				current = []map[string]interface{}{embed}
			case []interface{}:
				result := []map[string]interface{}{}

				for _, l := range link {
					embed, err := e.get(req, l.(map[string]interface{}))
					if err != nil {
						log.Fatal(err)
						continue
					}

					result = append(result, embed)
				}

				embedded[spec[i]] = result
				current = result
			default:
				continue
			}
		}
	}

	b, err = json.Marshal(data)
	if err != nil {
		return resp, err
	}

	resp.Body = ioutil.NopCloser(bytes.NewReader(b))

	return resp, nil
}

func stripSpec(req *http.Request) (*http.Request, []string, error) {
	with := req.URL.Query().Get("with")
	spec := strings.Split(with, ".")

	q := req.URL.Query()
	q.Del("with")
	u := url.URL{
		Scheme:   req.URL.Scheme,
		Opaque:   req.URL.Opaque,
		User:     req.URL.User,
		Host:     req.URL.Host,
		Path:     req.URL.Path,
		RawQuery: q.Encode(),
		Fragment: req.URL.Fragment,
	}

	r, err := http.NewRequest(req.Method, u.String(), nil)
	if err != nil {
		return r, spec, err
	}
	r.Header = req.Header

	return r, spec, nil
}

func (e *EmbedTransport) get(base *http.Request, link map[string]interface{}) (map[string]interface{}, error) {
	transport := e.RoundTripper
	if transport == nil {
		transport = http.DefaultTransport
	}

	href, ok := link["href"].(string)
	if !ok {
		return nil, errors.New("href not found")
	}

	uri, err := url.Parse(href)
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
