package main

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/ichiban/jesi/balance"
	"github.com/ichiban/jesi/cache"
	"github.com/ichiban/jesi/conditional"
	"github.com/ichiban/jesi/embed"
	log "github.com/sirupsen/logrus"
)

var version string

func main() {
	var port int
	var backends balance.BackendPool
	var max uint64
	var verbose bool

	flag.IntVar(&port, "port", 8080, "port number")
	flag.Var(&backends, "backend", "backend servers")
	flag.Uint64Var(&max, "max", 64, "max cache size in MB")
	flag.BoolVar(&verbose, "verbose", false, "log extra information")
	flag.Parse()

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	go backends.Run(nil)

	handler := http.Handler(&balance.Handler{
		BackendPool: &backends,
	})
	handler = &cache.Handler{
		Next: handler,
		Cache: cache.Cache{
			Max: max * 1024 * 1024,
		},
	}
	handler = &embed.Handler{
		Next: handler,
	}
	handler = &conditional.Handler{
		Next: handler,
	}

	log.WithFields(log.Fields{
		"version": version,
		"port":    port,
		"max":     max,
		"verbose": verbose,
	}).Info("Started a server")

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}
	if err := server.ListenAndServe(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("The server failed")
	}
}
