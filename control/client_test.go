package control

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClient_Run(t *testing.T) {
	testCases := []struct {
		in  []string
		out []*Event
	}{
		{
			in: []string{
				":comment",
			},
			out: []*Event{},
		},
		{
			in: []string{
				": multi-line\n: comment\n\n",
			},
			out: []*Event{},
		},
		{
			in: []string{
				"event: foo\n\n",
			},
			out: []*Event{
				{
					Event: "foo",
				},
			},
		},
		{
			in: []string{
				"event: bar\n",
				"data: baz\n\n",
			},
			out: []*Event{
				{
					Event: "bar",
					Data:  []byte("baz"),
				},
			},
		},
		{
			in: []string{
				"data: foo\ndata:bar\ndata: baz\n\n",
			},
			out: []*Event{
				{
					Data: []byte("foobarbaz"),
				},
			},
		},
		{
			in: []string{
				"id: foo\nretry: 1\ndata:event with id and retry\n\n",
			},
			out: []*Event{
				{
					ID:   "foo",
					Data: []byte("event with id and retry"),
				},
			},
		},
	}

	for i, ts := range testCases {
		s := httptest.NewServer(&testServer{
			T:      t,
			Chunks: ts.in,
		})

		u, err := url.Parse(s.URL)
		if err != nil {
			t.Errorf("Faild to parse URL: %s", s.URL)
		}

		events := make(chan *Event)
		quit := make(chan struct{})

		client := Client{
			URL:      u,
			Retry:    time.Second,
			Interval: time.Second,
			Events:   events,
			Quit:     quit,
		}
		go client.Run(nil)

		for j, expected := range ts.out {
			select {
			case got := <-events:
				if expected.ID != got.ID {
					t.Errorf("(%d, %d) expected: %s, got: %s", i, j, expected.ID, got.ID)
				}
				if expected.Event != got.Event {
					t.Errorf("(%d, %d) expected: %s, got: %s", i, j, expected.Event, got.Event)
				}
				if string(expected.Data) != string(got.Data) {
					t.Errorf("(%d, %d) expected: %s, got: %s", i, j, string(expected.Data), string(got.Data))
				}
				if got.ID != "" && client.LastEventID != got.ID {
					t.Errorf("(%d, %d) expected LastEventID=%s, got ID=%s", i, j, client.LastEventID, got.ID)
				}
			case <-time.After(time.Second):
				t.Errorf("(%d, %d) timeout", i, j)
			}
		}
		quit <- struct{}{}
		s.Close()
	}
}

func TestClient_String(t *testing.T) {
	testCases := []struct {
		client Client
		string string
	}{
		{
			client: Client{
				URL: &url.URL{
					Scheme: "http",
					Host:   "www.example.com",
					Path:   "/",
				},
			},
			string: "http://www.example.com/",
		},
	}

	for _, ts := range testCases {
		got := ts.client.String()
		if ts.string != got {
			t.Errorf("expected: %s, got: %s", ts.string, got)
		}
	}
}

func TestClient_Flush(t *testing.T) {
	testCases := []struct {
		buffer []byte
	}{
		{
			buffer: []byte("foo bar"),
		},
	}

	for i, ts := range testCases {
		client := &Client{
			URL: &url.URL{
				Scheme: "http",
				Host:   "www.example.com",
				Path:   "/",
			},
		}
		client.Transport = &testTransport{
			f: func(req *http.Request) (*http.Response, error) {
				b, err := ioutil.ReadAll(req.Body)
				if err != nil {
					t.Errorf("(%d) failed to read all: %v", i, err)
				}
				if string(ts.buffer) != string(b) {
					t.Errorf("(%d) expected: %s, got: %s", i, string(ts.buffer), string(b))
				}

				return &http.Response{
					StatusCode: http.StatusOK,
				}, nil
			},
		}
		if _, err := client.Buffer.Write(ts.buffer); err != nil {
			t.Errorf("(%d) failed to write: %v", i, err)
		}
		if err := client.Flush(); err != nil {
			t.Errorf("(%d) failed to flush: %v", i, err)
		}
	}
}

func TestClientPool_String(t *testing.T) {
	testCases := []struct {
		urls   []url.URL
		string string
	}{
		{
			urls:   []url.URL{},
			string: "[]",
		},
		{
			urls: []url.URL{
				{
					Scheme: "http",
					Host:   "www.example.com",
					Path:   "/foo/bar",
				},
			},
			string: "[http://www.example.com/foo/bar]",
		},
		{
			urls: []url.URL{
				{
					Scheme: "http",
					Host:   "www.example.com",
					Path:   "/foo/bar",
				},
				{
					Scheme: "http",
					Host:   "localhost",
					Path:   "/events",
				},
			},
			string: "[http://www.example.com/foo/bar http://localhost/events]",
		},
	}

	for i, ts := range testCases {
		var p ClientPool

		for _, u := range ts.urls {
			u := u
			p.Clients = append(p.Clients, &Client{
				URL: &u,
			})
		}

		if ts.string != p.String() {
			t.Errorf("(%d) expected: %s, got: %s", i, ts.string, p.String())
		}
	}
}

func TestClientPool_Set(t *testing.T) {
	testCases := []struct {
		urls []string
	}{
		{
			urls: []string{
				"http://localhost/events",
			},
		},
		{
			urls: []string{
				"http://localhost/events",
				"http://www.example.com/foo/bar",
			},
		},
	}

	for i, ts := range testCases {
		var p ClientPool
		for _, u := range ts.urls {
			p.Set(u)
		}
		if len(ts.urls) != len(p.Clients) {
			t.Errorf("(%d) expected: %d, got: %d", i, len(ts.urls), len(p.Clients))
		}
		for j, u := range ts.urls {
			c := p.Clients[j]
			if u != c.URL.String() {
				t.Errorf("(%d, %d) expected: %s, got: %s", i, j, u, c.URL.String())
			}
			if 10*time.Second != c.Retry {
				t.Errorf("(%d, %d) expected: %s, got: %s", i, j, 10*time.Second, c.Retry)
			}
			if time.Minute != c.Interval {
				t.Errorf("(%d, %d) expected: %s, got: %s", i, j, 10*time.Second, c.Interval)
			}
		}
	}
}

func TestClientPool_Add(t *testing.T) {
	testCases := []struct {
		clients []*Client
	}{
		{
			clients: []*Client{
				{
					URL: &url.URL{
						Scheme: "http",
						Host:   "localhost",
						Path:   "/events",
					},
				},
			},
		},
		{
			clients: []*Client{
				{
					URL: &url.URL{
						Scheme: "http",
						Host:   "localhost",
						Path:   "/events",
					},
				},
				{
					URL: &url.URL{
						Scheme: "http",
						Host:   "www.example.com",
						Path:   "/foo/bar",
					},
				},
			},
		},
	}

	for i, ts := range testCases {
		var p ClientPool
		for _, c := range ts.clients {
			p.Add(c)
		}
		if len(ts.clients) != len(p.Clients) {
			t.Errorf("(%d) expected: %d, got: %d", i, len(ts.clients), len(p.Clients))
		}
		for j, expected := range ts.clients {
			got := p.Clients[j]
			if expected != got {
				t.Errorf("(%d, %d) expected: %v, got: %v", i, j, expected, got)
			}
		}
	}
}

func TestClientPool_Run(t *testing.T) {
	testCases := []struct {
		clients []*Client
	}{
		{
			clients: []*Client{
				{
					URL: &url.URL{
						Scheme: "http",
						Host:   "localhost",
						Path:   "/events",
					},
				},
			},
		},
		{
			clients: []*Client{
				{
					URL: &url.URL{
						Scheme: "http",
						Host:   "localhost",
						Path:   "/events",
					},
				},
				{
					URL: &url.URL{
						Scheme: "http",
						Host:   "www.example.com",
						Path:   "/foo/bar",
					},
				},
			},
		},
	}

	for i, ts := range testCases {
		var p ClientPool
		p.Clients = ts.clients
		ch := make(chan struct{})
		go p.Run(ch)
		for j := range ts.clients {
			select {
			case <-ch:
			case <-time.After(time.Second):
				t.Errorf("(%d, %d) timeout", i, j)
			}
		}
	}
}

func TestClientPool_Levels(t *testing.T) {
	var p ClientPool
	ls := p.Levels()
	for i, l := range logrus.AllLevels {
		if l != ls[i] {
			t.Errorf("(%d) expected: %s, got: %s", i, l, ls[i])
		}
	}
}

func TestClientPool_Fire(t *testing.T) {
	testCases := []struct {
		entries []logrus.Entry
		buffer  string
	}{
		{
			entries: []logrus.Entry{},
			buffer:  "",
		},
		{
			entries: []logrus.Entry{
				{
					Level:   logrus.InfoLevel,
					Message: "foo",
				},
			},
			buffer: "{\"level\":\"info\",\"msg\":\"foo\",\"time\":\"0001-01-01T00:00:00Z\"}\n",
		},
	}

	for i, ts := range testCases {
		p := ClientPool{
			Clients: []*Client{
				{
					URL: &url.URL{
						Scheme: "http",
						Host:   "localhost",
						Path:   "/events",
					},
				},
				{
					URL: &url.URL{
						Scheme: "http",
						Host:   "www.example.com",
						Path:   "/foo/bar",
					},
				},
			},
		}

		for j, e := range ts.entries {
			e := e
			if err := p.Fire(&e); err != nil {
				t.Errorf("(%d, %d) failed to fire: %v", i, j, err)
			}
		}

		for j, c := range p.Clients {
			s := c.Buffer.String()
			if ts.buffer != s {
				t.Errorf("(%d, %d) expected: %v, got: %v", i, j, []byte(ts.buffer), []byte(s))
			}
		}
	}
}

type testServer struct {
	*testing.T
	Chunks []string
}

var _ http.Handler = (*testServer)(nil)

func (s *testServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)

	for _, chunk := range s.Chunks {
		if _, err := fmt.Fprint(w, chunk); err != nil {
			s.Errorf("Couldn't write to response: %v", err)
		}
	}
}

type testTransport struct {
	f func(*http.Request) (*http.Response, error)
}

var _ http.RoundTripper = (*testTransport)(nil)

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.f(req)
}
