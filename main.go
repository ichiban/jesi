package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/ichiban/jesi/cache"
	"github.com/ichiban/jesi/embed"
)

func main() {
	var port int
	var backend string
	var max uint64

	flag.IntVar(&port, "port", 8080, "port number")
	flag.StringVar(&backend, "backend", "http://localhost:3000", "backend server")
	flag.Uint64Var(&max, "max", 0, "max cache size in bytes")
	flag.Parse()

	log.Printf("port: %d", port)
	log.Printf("backend: %s", backend)
	log.Printf("size: %d", max)

	uri, err := url.Parse(backend)
	if err != nil {
		log.Fatal(err)
	}

	handler := httputil.NewSingleHostReverseProxy(uri)
	transport := &cache.Transport{}
	transport.Cache.Max = max
	handler.Transport = &embed.Transport{RoundTripper: transport}
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
