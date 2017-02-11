package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/ichiban/jesi/cache"
	"github.com/ichiban/jesi/conditional"
	"github.com/ichiban/jesi/embed"
)

var version string

func main() {
	log.Printf("version: %s", version)

	var port int
	var backend string
	var max uint64

	flag.IntVar(&port, "port", 8080, "port number")
	flag.StringVar(&backend, "backend", "http://localhost:3000", "backend server")
	flag.Uint64Var(&max, "max", 64, "max cache size in MB")
	flag.Parse()

	log.Printf("port: %d", port)
	log.Printf("backend: %s", backend)
	log.Printf("max: %d", max)

	uri, err := url.Parse(backend)
	if err != nil {
		log.Fatal(err)
	}

	handler := httputil.NewSingleHostReverseProxy(uri)
	handler.Transport = &conditional.Transport{
		RoundTripper: &embed.Transport{
			RoundTripper: &cache.Transport{
				Cache: cache.Cache{
					Max: max * 1024 * 1024,
				},
			},
		},
	}
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
