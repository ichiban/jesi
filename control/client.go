package control

import (
	"bufio"
	"bytes"
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
	sync.RWMutex

	URL  *url.URL
	Quit chan struct{}

	// in
	LastEventID string
	Retry       time.Duration
	Events      chan *Event

	// out
	Interval time.Duration
}

// Run runs the client.
func (c *Client) Run(ch chan<- struct{}) {
	log.WithFields(log.Fields{
		"control": c.URL,
	}).Info("Will respond to events from a control server")

	if ch != nil {
		ch <- struct{}{}
	}

	wq := make(chan struct{})
	go c.write(wq)
	rq := make(chan struct{})
	go c.read(rq)
	<-c.Quit
	wq <- struct{}{}
	rq <- struct{}{}
}

func (c *Client) write(q <-chan struct{}) {
	for {
		c.RLock()
		d := c.Interval
		c.RUnlock()
		select {
		case <-time.After(d):
			if err := c.Flush(); err != nil {
				log.WithFields(log.Fields{
					"error": err,
				}).Warn("Failed to flush")
			}
		case <-q:
			return
		}
	}
}

func (c *Client) read(q <-chan struct{}) {
	for {
		c.readEventStream(q)
		c.RLock()
		d := c.Retry
		c.RUnlock()
		time.Sleep(d)
	}
}

func (c *Client) readEventStream(q <-chan struct{}) {
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

	done := make(chan struct{})
	go func() {
		for {
			e, err := c.readEvent(resp.Body)
			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
				}).Debug("Failed to readEventStream event stream")
				break
			}

			c.Events <- e
		}
		done <- struct{}{}
	}()

	for {
		select {
		case <-done:
			return
		case <-q:
			return
		}
	}
}

func (c *Client) String() string {
	return c.URL.String()
}

// Flush sends the contents of log buffer to the control server.
func (c *Client) Flush() error {
	c.Lock()
	defer c.Unlock()

	req, err := http.NewRequest(http.MethodPost, c.URL.String(), c)
	if err != nil {
		log.WithFields(log.Fields{
			"client": c.String(),
		}).Debug("Failed to create a new request")

		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusAlreadyReported:
	default:
		log.WithFields(log.Fields{
			"control": c.URL,
			"status":  resp.StatusCode,
		}).Warn("Failed to report")
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
		if len(l) == 0 {
			break
		}

		ms := commentOrField.FindStringSubmatch(l)
		field := ms[1]
		value := ms[2]

		switch field {
		case "": // comment
			continue
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
	Events    chan *Event
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
		URL:      u,
		Retry:    10 * time.Second,
		Interval: time.Minute,
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
func (p *ClientPool) Run(ch chan<- struct{}) {
	var wg sync.WaitGroup
	for _, c := range p.Clients {
		wg.Add(1)
		go func(c *Client) {
			c.Events = p.Events
			c.Run(ch)
			wg.Done()
		}(c)
	}
	wg.Wait()
}

// Levels returns log levels that control servers are aware of.
func (p *ClientPool) Levels() []log.Level {
	return log.AllLevels
}

// Fire put log entries in JSON format to the control clients' buffers.
func (p *ClientPool) Fire(e *log.Entry) error {
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
