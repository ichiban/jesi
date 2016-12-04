package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/ichiban/jesi/cache"
	"github.com/ichiban/jesi/embed"
)

func main() {
	addr := ":8080"
	uri, err := url.Parse("http://127.0.0.1:3000")
	if err != nil {
		log.Fatal(err)
	}
	handler := httputil.NewSingleHostReverseProxy(uri)
	handler.Transport = &embed.Transport{&cache.Transport{}}
	server := http.Server{
		Addr:    addr,
		Handler: handler,
	}
	server.ListenAndServe()
}
