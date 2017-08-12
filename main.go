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
	var proxy Proxy
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

	controls.Backends = &backends
	controls.Store = &store
	go controls.Run()

	handler := http.Handler(&balance.Handler{
		BackendPool: &backends,
	})
	handler = &cache.Handler{
		Next:  handler,
		Store: &store,
	}
	handler = &embed.Handler{
		Next: handler,
	}
	handler = &conditional.Handler{
		Next: handler,
	}

	log.WithFields(log.Fields{
		"version": version,
		"port":    proxy.Port,
		"max":     store.Max,
		"verbose": verbose,
	}).Info("Started a server")

	proxy.Handler = handler
	proxy.Run()
}

type Proxy struct {
	Port    int
	Handler http.Handler
}

func (p *Proxy) Run() {
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", p.Port),
		Handler: p.Handler,
	}

	if err := server.ListenAndServe(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("The server failed")
	}
}
