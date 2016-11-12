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

	for k, v := range links {
		if k != spec[0] {
			continue
		}

		switch v := v.(type) {
		case map[string]interface{}:
			data, err := e.embed(req, v)
			if err != nil {
				continue
			}

			embedded[k] = data
		case []interface{}:
			var arr []map[string]interface{}
			for _, v := range v {
				data, err := e.embed(req, v.(map[string]interface{}))
				if err != nil {
					continue
				}

				arr = append(arr, data)
			}
			embedded[k] = arr
		default:
			continue
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

func (e *EmbedTransport) embed(parent *http.Request, link map[string]interface{}) (map[string]interface{}, error) {
	href, ok := link["href"].(string)
	if !ok {
		return nil, errors.New("href not found")
	}

	uri, err := url.Parse(href)
	if err != nil {
		return nil, err
	}

	_, spec, err := stripSpec(parent)
	if err != nil {
		return nil, err
	}

	q := uri.Query()
	q["with"] = spec[1:]
	uri.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, parent.URL.ResolveReference(uri).String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header = parent.Header

	resp, err := e.RoundTrip(req)
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
