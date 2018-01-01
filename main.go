package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"

	log "github.com/sirupsen/logrus"

	"github.com/ichiban/jesi/balance"
	"github.com/ichiban/jesi/cache"
	"github.com/ichiban/jesi/conditional"
	"github.com/ichiban/jesi/control"
	"github.com/ichiban/jesi/embed"
	"github.com/ichiban/jesi/forward"
	"github.com/ichiban/jesi/transaction"
)

var version string

func main() {
	var profile string
	var proxy ReverseProxy
	var node balance.Node
	var backends balance.BackendPool
	var store cache.Store
	var verbose bool
	var secret string

	flag.StringVar(&profile, "profile", "", "run debug profiler")
	flag.IntVar(&proxy.Port, "port", 8080, "port number")
	flag.Var(&node, "node", "node identifier (e.g. _jesi)")
	flag.Var(&backends, "backend", "backend servers")
	flag.Uint64Var(&store.Max, "max", 64*1024*1024, "max cache size in bytes")
	flag.UintVar(&store.Sample, "sample", 3, "sample size for cache eviction")
	flag.BoolVar(&verbose, "verbose", false, "log extra information")
	flag.StringVar(&secret, "secret", "", "bearer token")
	flag.Parse()

	if profile != "" {
		go func() {
			log.WithFields(log.Fields{
				"host": profile,
			}).Info("Start a debug profiler")
			if err := http.ListenAndServe(profile, nil); err != nil {
				log.WithFields(log.Fields{
					"host":  profile,
					"error": err,
				}).Error("Failed to profile")
			}
		}()
	}

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	go backends.Run(nil)

	log.WithFields(log.Fields{
		"version": version,
		"port":    proxy.Port,
		"node":    &node,
		"max":     store.Max,
		"verbose": verbose,
	}).Info("Start a server")

	proxy.Node = &node
	proxy.Backends = &backends
	proxy.Store = &store
	proxy.Secret = secret
	proxy.Run()
}

// ReverseProxy handles requests from downstream.
type ReverseProxy struct {
	Node     *balance.Node
	Port     int
	Backends *balance.BackendPool
	Store    *cache.Store
	Secret   string
}

// Run runs the reverse proxy.
func (p *ReverseProxy) Run() {
	var handler http.Handler
	handler = &forward.Handler{
		Transport: http.DefaultTransport,
	}
	handler = &transaction.Handler{
		Type: "up",
		Next: handler,
	}
	handler = &balance.Handler{
		Node:        p.Node,
		BackendPool: p.Backends,
		Next:        handler,
	}
	handler = &cache.Handler{
		Next:  handler,
		Store: p.Store,
	}
	handler = &transaction.Handler{
		Type: "internal",
		Next: handler,
	}
	handler = &control.Handler{
		Store:  p.Store,
		Secret: p.Secret,
		Next:   handler,
	}
	handler = &embed.Handler{
		Next: handler,
	}
	handler = &conditional.Handler{
		Next: handler,
	}
	handler = &transaction.Handler{
		Type: "down",
		Next: handler,
	}
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", p.Port),
		Handler: normalizeURL(handler),
	}

	if err := server.ListenAndServe(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("The server failed")
	}
}

func normalizeURL(h http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			r.URL.Scheme = "http"
		} else {
			r.URL.Scheme = "https"
		}
		r.URL.Host = r.Host
		h.ServeHTTP(w, r)
	})
}
