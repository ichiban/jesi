package control

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/ichiban/jesi/balance"
	"github.com/ichiban/jesi/cache"
)

type Client struct {
	http.Client

	Backends *balance.BackendPool
	Store    *cache.Store

	URL *url.URL

	LastEventID string
	Retry       time.Duration

	Events chan *Event
}

func (c *Client) Run() {
	log.WithFields(log.Fields{
		"control": c.URL,
	}).Info("Will respond to events from a control server")

	for {
		resp, err := c.Client.Get(c.URL.String())
		if err != nil {
			log.WithFields(log.Fields{
				"control": c.URL,
			}).Warn("Control server doesn't respond")

			if c.Retry == 0 {
				c.Retry = time.Duration(10) * time.Second
			}

			log.WithFields(log.Fields{
				"control":  c.URL,
				"interval": c.Retry,
			}).Info("Will connect to a control server after an interval")

			time.Sleep(c.Retry)
			continue
		}

		log.WithFields(log.Fields{
			"control": c.URL,
			"status":  resp.StatusCode,
		}).Info("Opened a stream from a control server")

		r := EventReader{Reader: bufio.NewReader(resp.Body)}
		for {
			e, err := r.ReadEvent()
			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
				}).Debug("Failed to read event stream")
				break
			}

			if e.ID != "" {
				c.LastEventID = e.ID
			}

			if e.Retry != 0 {
				c.Retry = e.Retry
			}

			c.Events <- e
		}

		resp.Body.Close()

		log.WithFields(log.Fields{
			"control": c.URL,
		}).Info("Closed a stream from a control server")

		log.WithFields(log.Fields{
			"control":  c.URL,
			"interval": c.Retry,
		}).Info("Will connect to a control server after an interval")

		time.Sleep(c.Retry)
	}
}

type Event struct {
	ID    string
	Type  string
	Data  []byte
	Retry time.Duration
}

var _ io.WriterTo = (*Event)(nil)

func (e *Event) WriteTo(w io.Writer) (int64, error) {
	var res int64

	if e.ID != "" {
		n, err := fmt.Fprintf(w, "id: %s\n", e.ID)
		if err != nil {
			return res, err
		}
		res += int64(n)
	}

	if e.Type != "" {
		n, err := fmt.Fprintf(w, "event: %s\n", e.Type)
		if err != nil {
			return res, err
		}
		res += int64(n)
	}

	if len(e.Data) != 0 {
		n, err := fmt.Fprintf(w, "data: %s\n", e.Data)
		if err != nil {
			return res, err
		}
		res += int64(n)
	}

	if e.Retry != 0 {
		n, err := fmt.Fprintf(w, "retry: %d\n", e.Retry.Nanoseconds()/1000000)
		if err != nil {
			return res, err
		}
		res += int64(n)
	}

	n, err := fmt.Fprint(w, "\n")
	if err != nil {
		return res, err
	}
	res += int64(n)

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return res, nil
}

var (
	CommentOrFieldPattern = regexp.MustCompile(`([\x{0000}-\x{0009}\x{000B}-\x{000C}\x{000E}-\x{0039}\x{003B}-\x{10FFFF}]*): ?([\x{0000}-\x{0009}\x{000B}-\x{000C}\x{000E}-\x{10FFFF}]*)`)
)

type EventReader struct {
	*bufio.Reader
}

func (r *EventReader) ReadEvent() (*Event, error) {
	var e *Event
	for {
		var l []byte
		var m bool
		var err error
		for {
			var b []byte
			b, m, err = r.ReadLine()
			if err != nil {
				return nil, err
			}
			l = append(l, b...)
			if !m {
				break
			}
		}

		// end of event
		if len(l) == 0 {
			if e == nil {
				continue
			}
			return e, nil
		}

		ms := CommentOrFieldPattern.FindAllSubmatch(l, -1)
		field := ms[0][1]
		value := ms[0][2]

		// comment
		if len(field) == 0 {
			continue
		}

		if e == nil {
			e = &Event{}
		}

		switch string(field) {
		case "event":
			e.Type = string(value)
		case "data":
			e.Data = append(e.Data, value...)
		case "id":
			e.ID = string(value)
		case "retry":
			retry, err := strconv.Atoi(string(value))
			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
				}).Debug("Failed to parse retry field")
			}
			e.Retry = time.Duration(retry) * time.Millisecond
		}
	}
}

type ClientPool struct {
	Clients []*Client

	Backends *balance.BackendPool
	Store    *cache.Store

	Events chan *Event
}

var _ flag.Value = (*ClientPool)(nil)

func (p *ClientPool) String() string {
	return fmt.Sprintf("%s", p.Clients)
}

func (p *ClientPool) Set(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}

	p.Clients = append(p.Clients, &Client{
		URL:    u,
		Events: p.Events,
	})

	return nil
}

func (p *ClientPool) Add(c *Client) {
	p.Clients = append(p.Clients, c)
	log.WithFields(log.Fields{
		"control": c.URL,
	}).Info("Added a control server")
}

func (p *ClientPool) Run() {
	var wg sync.WaitGroup
	for _, c := range p.Clients {
		c.Backends = p.Backends
		c.Store = p.Store
		wg.Add(1)
		go func() {
			c.Run()
			wg.Done()
		}()
	}
	wg.Wait()
}
