package control

import (
	"bufio"
	"bytes"
	"encoding/json"
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
)

// Client talks to a control server and receives events which will change states of the running process.
type Client struct {
	http.Client
	bytes.Buffer
	sync.Mutex

	URL *url.URL

	LastEventID string
	Retry       time.Duration

	// test seam
	Handler
}

// Handler handles events.
type Handler interface {
	Handle(*Event)
}

var _ Handler = (*Client)(nil)

// Run runs the client.
func (c *Client) Run(q chan struct{}) {
	log.WithFields(log.Fields{
		"control": c.URL,
	}).Debug("Will respond to events from a control server")

	// Run once as soon as possible.
	c.readEventStream(q)

	for {
		select {
		case <-q:
			break
		case <-time.After(c.Retry):
			c.readEventStream(q)
		}
	}
}

func (c *Client) readEventStream(q chan struct{}) {
	resp, err := c.Client.Get(c.URL.String())
	if err != nil {
		log.WithFields(log.Fields{
			"control": c.URL,
		}).Warn("Control server doesn't respond")

		if c.Retry == 0 {
			c.Retry = time.Duration(10) * time.Second
		}

		return
	}
	defer resp.Body.Close()

	log.WithFields(log.Fields{
		"control": c.URL,
		"status":  resp.StatusCode,
	}).Info("Opened a stream from a control server")

	go func() {
		for {
			e, err := c.readEvent(resp.Body)
			if err == io.EOF {
				break
			}
			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
				}).Debug("Failed to readEventStream event stream")
				break
			}

			if c.Handler == nil {
				c.Handler = c
			}

			c.Handler.Handle(e)
		}
		q <- struct{}{}
	}()

	q <- <-q
}

func (c *Client) String() string {
	return c.URL.String()
}

// Handle handles events.
func (c *Client) Handle(e *Event) {
	log.WithFields(log.Fields{
		"id":    e.ID,
		"event": e.Event,
		"data":  e.Data,
	}).Debug("Will handle an event")

	switch e.Event {
	case "report":
		var args struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(e.Data, &args); err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Failed to unmarshal report args")
			break
		}
		u, err := url.Parse(args.URL)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Failed to parse report url")
			break
		}
		if err := c.Report(c.URL.ResolveReference(u)); err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Failed to report")
			break
		}
	}
}

// Report sends the contents of log buffer to the control server.
func (c *Client) Report(u *url.URL) error {
	c.Lock()
	defer c.Unlock()

	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(c.Buffer.Bytes()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	if _, err := c.Do(req); err != nil {
		return err
	}

	c.Reset()

	return nil
}

// Event represents an event from the control server.
type Event struct {
	ID    string
	Event string
	Data  []byte
}

var (
	commentOrField = regexp.MustCompile(`([\x{0000}-\x{0009}\x{000B}-\x{000C}\x{000E}-\x{0039}\x{003B}-\x{10FFFF}]*): ?([\x{0000}-\x{0009}\x{000B}-\x{000C}\x{000E}-\x{10FFFF}]*)`)
)

func (c *Client) readEvent(r io.Reader) (*Event, error) {
	s := bufio.NewScanner(r)
	var e Event
	for s.Scan() {
		l := s.Text()

		// end of event
		if l == "" {
			break
		}

		ms := commentOrField.FindStringSubmatch(l)

		var field, value string
		if len(ms) == 0 {
			field = l
			value = ""
		} else {
			field = ms[1]
			value = ms[2]
		}

		switch field {
		case "event":
			e.Event = string(value)
		case "data":
			e.Data = append(e.Data, value...)
		case "id":
			id := string(value)
			e.ID = id
			c.LastEventID = id
		case "retry":
			retry, err := strconv.Atoi(string(value))
			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
				}).Debug("Failed to parse retry field")
			}
			c.Retry = time.Duration(retry) * time.Millisecond
		}
	}
	return &e, nil
}

// ClientPool is a set of clients.
type ClientPool struct {
	Clients   []*Client
	Formatter log.JSONFormatter
}

var _ flag.Value = (*ClientPool)(nil)
var _ log.Hook = (*ClientPool)(nil)

func (p *ClientPool) String() string {
	return fmt.Sprintf("%s", p.Clients)
}

// Set adds a brand new client based on the given URL.
func (p *ClientPool) Set(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}

	p.Add(&Client{
		URL:   u,
		Retry: 10 * time.Second,
	})

	return nil
}

// Add adds a client to the pool.
func (p *ClientPool) Add(c *Client) {
	p.Clients = append(p.Clients, c)
	log.WithFields(log.Fields{
		"control": c.URL,
	}).Info("Added a control server")
}

// Run runs all the clients in the pool.
func (p *ClientPool) Run(q chan struct{}) {
	var wg sync.WaitGroup
	for _, c := range p.Clients {
		wg.Add(1)
		go func(c *Client) {
			c.Run(q)
			wg.Done()
		}(c)
	}
	wg.Wait()
}

// Levels returns log levels that control servers are aware of.
func (p *ClientPool) Levels() []log.Level {
	return []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
		log.InfoLevel,
	}
}

// Fire put log entries in JSON format to the control clients' buffers.
func (p *ClientPool) Fire(e *log.Entry) error {
	for k, v := range e.Data {
		if v, ok := v.(fmt.Stringer); ok {
			e.Data[k] = v.String()
		}
	}

	b, err := p.Formatter.Format(e)
	if err != nil {
		return err
	}

	for _, c := range p.Clients {
		c.Lock()
		_, err := c.Write(b)
		c.Unlock()
		if err != nil {
			return err
		}
	}

	return nil
}
