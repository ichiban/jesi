package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/ichiban/jesi/embed"
	"github.com/ichiban/jesi/cache"
)

func main() {
	addr := ":8080"
	uri, _ := url.Parse("http://127.0.0.1:3000")
	handler := httputil.NewSingleHostReverseProxy(uri)
	handler.Transport = &embed.Transport{&cache.Transport{}}
	server := http.Server{
		Addr:    addr,
		Handler: handler,
	}
	server.ListenAndServe()
}
