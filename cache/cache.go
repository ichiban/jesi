package cache

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"io/ioutil"
	"bytes"
)

type Cache struct {
	sync.RWMutex
	Primary map[PrimaryKey]*PrimaryEntry
}

func (c *Cache) Set(req *http.Request, resp *http.Response) error {
	c.init()

	c.Lock()
	defer c.Unlock()

	pKey := NewPrimaryKey(req)
	pe, ok := c.Primary[pKey]
	if !ok {
		pe = NewPrimaryEntry(resp)
		c.Primary[pKey] = pe
	}

	sKey := NewSecondaryKey(pe, req)

	se, err := NewSecondaryEntry(resp)
	if err != nil {
		return err
	}

	pe.Secondary[sKey] = se

	return nil
}

func (c *Cache) Get(req *http.Request) *http.Response {
	c.init()

	c.RLock()
	defer c.RUnlock()

	pKey := NewPrimaryKey(req)
	pe, ok := c.Primary[pKey]
	if !ok {
		return nil
	}

	sKey := NewSecondaryKey(pe, req)
	se := pe.Secondary[sKey]
	if se == nil {
		return nil
	}

	return se.Response()
}

func (c *Cache) init() {
	c.Lock()
	defer c.Unlock()

	if c.Primary == nil {
		c.Primary = make(map[PrimaryKey]*PrimaryEntry)
	}
}

func (c *Cache) Clear() {
	c.Lock()
	defer c.Unlock()

	c.Primary = make(map[PrimaryKey]*PrimaryEntry)
}

type PrimaryKey struct {
	Host string
	Path string
}

func NewPrimaryKey(req *http.Request) PrimaryKey {
	return PrimaryKey{
		Host: req.URL.Host,
		Path: req.URL.Path,
	}
}

type PrimaryEntry struct {
	Fields    []string
	Secondary map[SecondaryKey]*SecondaryEntry
}

func NewPrimaryEntry(resp *http.Response) *PrimaryEntry {
	var fields []string

	for _, vary := range resp.Header["Vary"] {
		for _, field := range strings.Split(vary, ",") {
			if field == "*" {
				return nil
			}
			fields = append(fields, http.CanonicalHeaderKey(strings.Trim(field, " ")))
		}
	}

	return &PrimaryEntry{
		Fields:    fields,
		Secondary: make(map[SecondaryKey]*SecondaryEntry),
	}
}

type SecondaryKey string

func NewSecondaryKey(pe *PrimaryEntry, req *http.Request) SecondaryKey {
	var keys []string
	for _, fields := range pe.Fields {
		fields := strings.Split(fields, ",")
		for _, field := range fields {
			keys = append(keys, strings.Trim(field, " "))
		}
	}

	vals := url.Values{}
	for _, key := range keys {
		var values []string
		for _, vals := range req.Header[key] {
			vals := strings.Split(vals, ",")
			for _, val := range vals {
				values = append(values, strings.Trim(val, " "))
			}
		}

		sort.Strings(values)
		for _, val := range values {
			vals.Add(key, val)
		}
	}

	return SecondaryKey(vals.Encode())
}

type SecondaryEntry struct {
	Header http.Header
	Body []byte
}

func NewSecondaryEntry(resp *http.Response) (*SecondaryEntry, error) {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	resp.Body = ioutil.NopCloser(bytes.NewReader(body))

	return &SecondaryEntry{
		Header: resp.Header,
		Body: body,
	}, nil
}

func (e *SecondaryEntry) Response() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: e.Header,
		Body: ioutil.NopCloser(bytes.NewReader(e.Body)),
	}
}
