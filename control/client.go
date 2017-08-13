package control

import (
	"bufio"
	"flag"
	"fmt"
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
	URL         *url.URL
	LastEventID string
	Retry       time.Duration
	Events      chan *Event
}

// Run runs the client.
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

		r := eventReader{Reader: bufio.NewReader(resp.Body)}
		for {
			e, err := r.readEvent()
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

		if err := resp.Body.Close(); err != nil {
			log.WithFields(log.Fields{
				"control": c.URL,
				"error":   err,
			}).Error("Failed to close body")
		}

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

func (c *Client) String() string {
	return c.URL.String()
}

// Event represents an event from the control server.
type Event struct {
	ID    string
	Event string
	Data  []byte
	Retry time.Duration
}

var (
	commentOrField = regexp.MustCompile(`([\x{0000}-\x{0009}\x{000B}-\x{000C}\x{000E}-\x{0039}\x{003B}-\x{10FFFF}]*): ?([\x{0000}-\x{0009}\x{000B}-\x{000C}\x{000E}-\x{10FFFF}]*)`)
)

type eventReader struct {
	*bufio.Reader
}

func (r *eventReader) readEvent() (*Event, error) {
	var e Event
	for {
		l, err := r.line()
		if err != nil {
			return nil, err
		}

		// end of event
		if len(l) == 0 {
			return &e, nil
		}

		ms := commentOrField.FindAllSubmatch(l, -1)
		field := ms[0][1]
		value := ms[0][2]

		switch string(field) {
		case "": // comment
			continue
		case "event":
			e.Event = string(value)
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

func (r *eventReader) line() ([]byte, error) {
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
	return l, nil
}

// ClientPool is a set of clients.
type ClientPool struct {
	Clients []*Client
	Events  chan *Event
}

var _ flag.Value = (*ClientPool)(nil)

func (p *ClientPool) String() string {
	return fmt.Sprintf("%s", p.Clients)
}

// Set adds a brand new client based on the given URL.
func (p *ClientPool) Set(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}

	p.Clients = append(p.Clients, &Client{
		URL: u,
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
func (p *ClientPool) Run() {
	var wg sync.WaitGroup
	for _, c := range p.Clients {
		wg.Add(1)
		go func(c *Client) {
			c.Events = p.Events
			c.Run()
			wg.Done()
		}(c)
	}
	wg.Wait()
}
