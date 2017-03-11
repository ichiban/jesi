package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/ichiban/jesi/balance"
	"github.com/ichiban/jesi/cache"
	"github.com/ichiban/jesi/conditional"
	"github.com/ichiban/jesi/embed"
)

var version string

func main() {
	log.Printf("version: %s", version)

	var port int
	var backends balance.BackendPool
	var max uint64

	flag.IntVar(&port, "port", 8080, "port number")
	flag.Var(&backends, "backend", "backend servers")
	flag.Uint64Var(&max, "max", 64, "max cache size in MB")
	flag.Parse()

	log.Printf("port: %d", port)
	log.Printf("backend: %s", &backends)
	log.Printf("max: %d", max)

	go backends.Run()

	server := http.Server{
		Addr: fmt.Sprintf(":%d", port),
		Handler: &conditional.Handler{
			Next: &embed.Handler{
				Next: &cache.Handler{
					Next: &balance.Handler{
						BackendPool: &backends,
					},
					Cache: cache.Cache{
						Max: max * 1024 * 1024,
					},
				},
			},
		},
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
