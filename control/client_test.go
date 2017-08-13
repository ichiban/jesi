package control

import (
	"fmt"
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

		client := Client{
			URL:    u,
			Retry:  time.Second,
			Events: events,
		}
		go client.Run()

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
				if expected.Retry != got.Retry {
					t.Errorf("(%d, %d) expected: %s, got: %s", i, j, expected.Retry, got.Retry)
				}
			case <-time.After(time.Second):
				t.Errorf("(%d, %d) timeout", i, j)
			}
		}

		s.Close()
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
