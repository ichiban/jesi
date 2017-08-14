package main

import (
	"flag"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/ichiban/jesi/balance"
	"github.com/ichiban/jesi/cache"
	"github.com/ichiban/jesi/conditional"
	"github.com/ichiban/jesi/control"
	"github.com/ichiban/jesi/embed"
)

var version string

func main() {
	var proxy ReverseProxy
	var backends balance.BackendPool
	var store cache.Store
	var verbose bool
	var controls control.ClientPool

	flag.IntVar(&proxy.Port, "port", 8080, "port number")
	flag.Var(&backends, "backend", "backend servers")
	flag.IntVar(&store.Max, "max", 64, "max cache size in MB")
	flag.BoolVar(&verbose, "verbose", false, "log extra information")
	flag.Var(&controls, "control", "control servers")
	flag.Parse()

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	go backends.Run(nil)

	events := make(chan *control.Event)
	controls.Events = events
	go controls.Run(nil)
	log.AddHook(&controls)

	log.WithFields(log.Fields{
		"version": version,
		"port":    proxy.Port,
		"max":     store.Max,
		"verbose": verbose,
	}).Info("Start a server")

	proxy.Backends = &backends
	proxy.Store = &store
	proxy.Run()
}

// ReverseProxy handles requests from downstream.
type ReverseProxy struct {
	Port     int
	Backends *balance.BackendPool
	Store    *cache.Store
}

// Run runs the reverse proxy.
func (p *ReverseProxy) Run() {
	var handler http.Handler
	handler = &balance.Handler{
		BackendPool: p.Backends,
	}
	handler = &cache.Handler{
		Next:  handler,
		Store: p.Store,
	}
	handler = &embed.Handler{
		Next: handler,
	}
	handler = &conditional.Handler{
		Next: handler,
	}
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", p.Port),
		Handler: handler,
	}

	if err := server.ListenAndServe(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("The server failed")
	}
}
