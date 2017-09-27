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
	var backends balance.BackendPool
	var store cache.Store
	var verbose bool
	var controls control.ClientPool

	flag.StringVar(&profile, "profile", "", "run debug profiler")
	flag.IntVar(&proxy.Port, "port", 8080, "port number")
	flag.Var(&backends, "backend", "backend servers")
	flag.IntVar(&store.Max, "max", 64, "max cache size in MB")
	flag.BoolVar(&verbose, "verbose", false, "log extra information")
	flag.Var(&controls, "control", "control servers")
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
	handler = &forward.Handler{
		Transport: http.DefaultTransport,
	}
	handler = &transaction.Handler{
		Type: "up",
		Next: handler,
	}
	handler = &balance.Handler{
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
		Handler: handler,
	}

	if err := server.ListenAndServe(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("The server failed")
	}
}
